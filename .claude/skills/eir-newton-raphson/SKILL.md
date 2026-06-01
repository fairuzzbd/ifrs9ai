---
name: eir-newton-raphson
description: Reference implementation Newton-Raphson IRR solver untuk EIR (Effective Interest Rate). Gunakan saat menulis EIR computation, amortisasi schedule, atau amendment re-estimation. Mencakup precision, convergence handling, edge cases, dan test scenarios.
---

# EIR — Newton-Raphson IRR Solver

## Definisi
EIR adalah rate `r` yang membuat **present value of expected future cashflows = initial carrying amount**.

```
Σ_{t=1..N} CF_t / (1 + r)^t = initial_outflow
                ↑
        (CF_t bisa negatif jika ada additional outflow)
```

Untuk standardisasi, kita normalisasi: initial outflow direpresentasikan sebagai `CF_0` negatif, lalu cari `r` sehingga:
```
NPV(r) = Σ_{t=0..N} CF_t / (1 + r)^t = 0
```

## Newton-Raphson
```
r_{n+1} = r_n − f(r_n) / f'(r_n)

f(r)  = Σ_{t=0..N} CF_t × (1 + r)^(-t)
f'(r) = Σ_{t=0..N} (-t) × CF_t × (1 + r)^(-(t+1))
```

## Parameter LOCKED (DEC-013)
- `tolerance = 1e-10` (decimal precision)
- `max_iter = 100`
- `r_initial = `:
  - Jika instrumen punya stated coupon: pakai `coupon_rate` (best seed)
  - Else: `0.10` (10%)
- Convergence: `|f(r_n)| < tolerance` **OR** `|r_n - r_{n-1}| < tolerance`

## Go reference implementation

```go
package eir

import (
    "errors"
    "github.com/shopspring/decimal"
)

const (
    tolerance = "0.0000000001"  // 1e-10
    maxIter   = 100
)

type CashFlow struct {
    Period int              // 0, 1, 2, ... N (in periods, e.g. months or days)
    Amount decimal.Decimal  // positive = inflow, negative = outflow
}

// SolveIRR returns the effective rate per period.
func SolveIRR(cashflows []CashFlow, initialGuess decimal.Decimal) (decimal.Decimal, error) {
    tol, _ := decimal.NewFromString(tolerance)
    r := initialGuess

    for i := 0; i < maxIter; i++ {
        f, fPrime := npvAndDeriv(cashflows, r)

        if f.Abs().LessThan(tol) {
            return r, nil
        }
        if fPrime.IsZero() {
            return decimal.Zero, errors.New("derivative is zero, cannot continue Newton-Raphson")
        }

        rNext := r.Sub(f.Div(fPrime))

        if rNext.Sub(r).Abs().LessThan(tol) {
            return rNext, nil
        }

        // Sanity: r must be > -1 (otherwise (1+r) <= 0 invalid)
        minusOne := decimal.NewFromInt(-1)
        if rNext.LessThanOrEqual(minusOne) {
            return decimal.Zero, errors.New("Newton-Raphson diverged: r ≤ -1")
        }

        r = rNext
    }
    return decimal.Zero, errors.New("Newton-Raphson did not converge within max_iter")
}

func npvAndDeriv(cfs []CashFlow, r decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
    one := decimal.NewFromInt(1)
    onePlusR := one.Add(r)

    npv := decimal.Zero
    dNpv := decimal.Zero

    for _, cf := range cfs {
        if cf.Period == 0 {
            npv = npv.Add(cf.Amount)
            // derivative term for t=0 is 0
            continue
        }
        t := decimal.NewFromInt(int64(cf.Period))
        // (1+r)^(-t)  --- use shopspring decimal Pow (only integer exponents supported,
        //                  so build manually via repeated division or use float fallback ONLY for power computation
        //                  followed by re-converting to decimal). For BLIPS prod we use a pure-decimal pow.
        powMinusT := decimalPow(onePlusR, -cf.Period)
        powMinusTPlus1 := decimalPow(onePlusR, -(cf.Period + 1))

        npv = npv.Add(cf.Amount.Mul(powMinusT))
        dNpv = dNpv.Add(t.Neg().Mul(cf.Amount).Mul(powMinusTPlus1))
    }
    return npv, dNpv
}

// decimalPow computes (base)^exp where exp is int (can be negative).
// Implementation uses repeated multiplication to maintain precision.
func decimalPow(base decimal.Decimal, exp int) decimal.Decimal {
    if exp == 0 {
        return decimal.NewFromInt(1)
    }
    if exp < 0 {
        return decimal.NewFromInt(1).Div(decimalPow(base, -exp))
    }
    result := decimal.NewFromInt(1)
    for i := 0; i < exp; i++ {
        result = result.Mul(base)
    }
    return result
}
```

## Amortisasi schedule generation

Setelah `r` (EIR) di-solve, generate schedule:
```
Outstanding_t = Outstanding_{t-1} - Principal_t
Interest_t    = Outstanding_{t-1} × EIR (or "monthly EIR" if periodic)
CF_t          = Principal_t + Interest_t
```

Schedule rows disimpan di `ecl.amortisasi_schedule`:
```sql
CREATE TABLE ecl.amortisasi_schedule (
  id                  UUID PRIMARY KEY,
  instrumen_id        UUID NOT NULL,
  schedule_version    INT NOT NULL,
  effective_from      TIMESTAMPTZ NOT NULL,
  effective_to        TIMESTAMPTZ NOT NULL DEFAULT 'infinity',
  period_number       INT NOT NULL,
  period_date         DATE NOT NULL,
  opening_outstanding NUMERIC(20,4),
  interest            NUMERIC(20,4),
  principal           NUMERIC(20,4),
  cashflow            NUMERIC(20,4),
  closing_outstanding NUMERIC(20,4),
  eir_used            NUMERIC(10,8) NOT NULL,
  -- ...audit fields...
  UNIQUE (instrumen_id, schedule_version, period_number)
);
```

## Amendment re-estimation

**WAJIB**: Insert new `schedule_version`, JANGAN UPDATE existing rows.

```go
func ApplyAmendment(instrumenID uuid.UUID, amendment AmendmentSpec) error {
    return uow.Run(ctx, func(tx Tx) error {
        // 1. Close previous schedule version
        if err := tx.Exec(`
            UPDATE ecl.amortisasi_schedule
            SET effective_to = $1, updated_at = now(), updated_by = $2
            WHERE instrumen_id = $3 AND effective_to = 'infinity'
        `, amendment.EffectiveDate, currentUser, instrumenID); err != nil {
            return err
        }

        // 2. Solve new EIR using remaining cashflow + amendment
        newCF := buildAmendmentCashflow(instrumenID, amendment)
        newEIR, err := SolveIRR(newCF, previousEIR)
        if err != nil {
            return fmt.Errorf("re-estimation failed: %w", err)
        }

        // 3. Generate new schedule with new version
        newVersion := currentMaxVersion(instrumenID) + 1
        newSchedule := generateSchedule(newCF, newEIR, newVersion, amendment.EffectiveDate)

        // 4. Insert new schedule (immutable, audit-grade)
        if err := tx.BulkInsert("ecl.amortisasi_schedule", newSchedule); err != nil {
            return err
        }

        // 5. Audit
        return writeAudit(tx, "AMORTISASI.AMEND", instrumenID, amendment)
    })
}
```

## POCI (Purchased or Originated Credit Impaired)
- Cashflow expectations sudah PD-adjusted di awal
- EIR yang dihitung = **credit-adjusted EIR**
- Tidak ada Stage 1 untuk POCI — langsung treated lifetime
- ECL movement direkognisi di P&L sejak inisiasi

## Test cases wajib

```go
func TestSolveIRR_StandardBond(t *testing.T) {
    // 5-year bond, coupon 8%, par 1,000,000
    cfs := []CashFlow{
        {0, dec("-1000000")},  // initial outflow
        {1, dec("80000")},
        {2, dec("80000")},
        {3, dec("80000")},
        {4, dec("80000")},
        {5, dec("1080000")},   // principal + last coupon
    }
    r, err := SolveIRR(cfs, dec("0.08"))
    assert.NoError(t, err)
    assert.InDelta(t, 0.08, r.InexactFloat64(), 1e-8)
}

func TestSolveIRR_IrregularCashflow(t *testing.T) {
    // Non-uniform CF (mid-life prepayment)
    // ...
}

func TestSolveIRR_PrepaymentAmendment(t *testing.T) {
    // After year 2 amendment changes remaining schedule
    // Verify new EIR is re-solved correctly
    // Verify old schedule rows are NOT mutated (only effective_to updated)
}

func TestSolveIRR_DoesNotConverge(t *testing.T) {
    // Pathological CF (e.g. multiple sign changes, no real root)
    cfs := []CashFlow{
        {0, dec("-100")},
        {1, dec("230")},
        {2, dec("-132")},
    }
    _, err := SolveIRR(cfs, dec("0.10"))
    assert.Error(t, err)
    // Should NOT silently return garbage
}

func TestSolveIRR_POCI(t *testing.T) {
    // PD-adjusted cashflow → credit-adjusted EIR
    // Compare against expected from FSD-APP-C example
}
```

## Anti-patterns
- ❌ `math.Pow(float64(...), ...)` di production path — loses precision.
- ❌ UPDATE row schedule lama saat amendment — destroys audit.
- ❌ Silent fallback `r = 0.10` jika non-converge — wajib error explicit.
- ❌ `decimal.RoundCash` di tiap iterasi — round HANYA di output final.

## Citation
- FSD-APP-C-ECL-EIR-v1.0.docx §4 (EIR + amortisasi)
- SoW_v1.4.docx §5 (IRR solver requirements)
- Decision Log DEC-013, DEC-016
