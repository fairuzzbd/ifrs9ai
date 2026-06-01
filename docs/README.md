# BLIPS IFRS 9 — Document Index

Folder ini berisi versi markdown dari seluruh dokumen project BLIPS IFRS 9 untuk Tugure. Dokumen markdown ini di-generate otomatis dari sumber Microsoft Word (.docx) di folder `../msword/` menggunakan pandoc, dan dimaksudkan untuk:

- AI agent ingestion (context lebih efisien dibanding parsing docx)
- Code review / pull request workflow di Git
- Quick search via grep across all docs
- Version control friendly (text-diff trackable)

## Folder Structure

```
blips-ifrs9-ai/
├── BLIPS_init_schema.sql     ← Ready-to-execute PostgreSQL 18 DDL
├── CLAUDE.md                  ← Project guide untuk Claude Code (jika ada)
├── docs/                      ← MARKDOWN — for AI agents & version control
│   ├── README.md (this file)
│   └── *.md
└── msword/                    ← SOURCE Microsoft Word (.docx) + PDF reference
    ├── *.docx
    └── Pefindo_Annual_Default_Study_2007-2025_EN.pdf
```

## Dokumen Utama (di folder docs/)

| Dokumen | Versi | Markdown File | Source (msword/) |
|---------|-------|---------------|------------------|
| **Decision Log** | v1.0 | [BLIPS_Decision_Log_v1.0.md](BLIPS_Decision_Log_v1.0.md) | `../msword/BLIPS_Decision_Log_v1.0.docx` |
| **Scope of Work** | v1.4 | [SoW_v1.4.md](SoW_v1.4.md) | `../msword/SoW_v1.4.docx` |
| **Business Requirements** | v1.1 | [BRD_BLIPS_IFRS9_v1.1.md](BRD_BLIPS_IFRS9_v1.1.md) | `../msword/BRD_BLIPS_IFRS9_v1.1.docx` |
| **FSD Master** | v1.1 | [FSD-BLIPS-MASTER-v1.1.md](FSD-BLIPS-MASTER-v1.1.md) | `../msword/FSD-BLIPS-MASTER-v1.1.docx` |
| **FSD Appendix A** | v1.1 | [FSD-APP-A-MasterData-SPPI-BM-v1.1.md](FSD-APP-A-MasterData-SPPI-BM-v1.1.md) | `../msword/FSD-APP-A-MasterData-SPPI-BM-v1.1.docx` |
| **FSD Appendix B** | v1.1 | [FSD-APP-B-TransactionLifecycle-v1.1.md](FSD-APP-B-TransactionLifecycle-v1.1.md) | `../msword/FSD-APP-B-TransactionLifecycle-v1.1.docx` |
| **FSD Appendix C** | v1.0 | [FSD-APP-C-ECL-EIR-v1.0.md](FSD-APP-C-ECL-EIR-v1.0.md) | `../msword/FSD-APP-C-ECL-EIR-v1.0.docx` |
| **FSD Appendix D** | v1.0 | [FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.md](FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.md) | `../msword/FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.docx` |
| **FSD Appendix E** | v1.0 | [FSD-APP-E-Reporting-v1.0.md](FSD-APP-E-Reporting-v1.0.md) | `../msword/FSD-APP-E-Reporting-v1.0.docx` |
| **ERD** | v1.2 | [ERD-BLIPS-IFRS9-v1.2.md](ERD-BLIPS-IFRS9-v1.2.md) | `../msword/ERD-BLIPS-IFRS9-v1.2.docx` |

## Source Reference (Non-Markdown)

| File | Format | Lokasi | Deskripsi |
|------|--------|--------|-----------|
| BLIPS_init_schema.sql | SQL | `../BLIPS_init_schema.sql` | Ready-to-execute DDL PostgreSQL 18 dengan sample seed data |
| Pefindo Annual Default Study | PDF | `../msword/Pefindo_Annual_Default_Study_2007-2025_EN.pdf` | Source PD aktual yang di-seed di SQL DDL |

## Reading Order untuk AI Agent / New Developer

Disarankan baca dengan urutan ini untuk membangun mental model yang benar:

1. **BLIPS_Decision_Log_v1.0.md** — pahami 6 keputusan kunci dan constraints
2. **SoW_v1.4.md** — pahami scope, modul, dan business flow
3. **BRD_BLIPS_IFRS9_v1.1.md** — pahami BR-IDs dan acceptance criteria
4. **FSD-BLIPS-MASTER-v1.1.md** — pahami tech stack, arsitektur, standards
5. **FSD-APP-A** sampai **FSD-APP-E** — detail teknis per modul (read on-demand per modul yang sedang dikerjakan)
6. **ERD-BLIPS-IFRS9-v1.2.md** — database schema lengkap
7. **BLIPS_init_schema.sql** — DDL ready-to-execute

## Konvensi Penomoran

- `BR-XXX-###` — Business Requirement ID (BRD)
- `FR-XXX-###` — Functional Requirement ID (FSD)
- `UC-XXX-###` — Use Case ID
- `ERR-XXX-####` — Error Code
- `DEC-###` — Decision Log ID

## Update Process

Bila Word (.docx) di-update di folder `msword/`, regenerate markdown dengan:

```bash
cd path/to/blips-ifrs9-ai
for f in msword/*.docx; do
  base=$(basename "$f" .docx)
  pandoc "$f" -t gfm --wrap=preserve -o "docs/${base}.md"
done
```

Markdown files di-overwrite — tidak perlu manual edit di `docs/` folder.
