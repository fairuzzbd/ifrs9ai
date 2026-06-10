// Package counterparty — PII utilities for counterparty module.
//
// MaskString and PIIResponse are used by the view_pii endpoint
// and the default GET/:id endpoint respectively.
//
// Rules (DEC-028):
//   - Default GET /:id response: PII masked (*** for any non-null field).
//   - GET /:id/pii: decrypted PII, permission counterparty.view_pii, audit COUNTERPARTY.VIEW_PII.
//   - List: never include PII.
//   - Export: PII masked (*** in CSV).
//   - Audit log: PII always REDACTED string, never plaintext.
package counterparty

// BuildPIIResponse creates a PIIResponse from decrypted PIIFields.
// Called only by the view_pii handler after permission check.
func BuildPIIResponse(id string, kode string, pii *PIIFields) PIIResponse {
	resp := PIIResponse{
		ID:               id,
		KodeCounterparty: kode,
	}
	if pii != nil {
		resp.NPWP = pii.NPWP
		resp.NomorRekening = pii.NomorRekening
		resp.KTP = pii.KTP
	}
	return resp
}
