// Package helpers — service coordinator (wire-up for all helpers).
//
// Services assembles PDLookupService, LGDLookupService, EADService, CCFLookupService,
// and BulkHelperService from a single DB pool.
//
// Usage (main.go):
//
//	svc := helpers.NewServices(db, auditWriter)
//	helpers.RegisterRoutes(router.Group("/api/v1/ecl"), helpers.NewHandler(svc))
package helpers

import (
	"database/sql"

	"blips-ifrs9.tugu-re.com/internal/audit"
)

// Services holds all helper service instances.
type Services struct {
	PD   PDLookupService
	LGD  LGDLookupService
	EAD  EADService
	CCF  CCFLookupService
	Bulk BulkHelperService

	// instrRepo is kept so NewHandler can wire the preview lister.
	instrRepo InstrumenSnapshotRepo
}

// NewServices builds all helpers services from a DB connection.
// auditWriter may be nil — audit writes are skipped if so.
func NewServices(db *sql.DB, auditWriter *audit.Writer) *Services {
	pdRepo    := NewDBPDRepository(db)
	lgdRepo   := NewDBLGDRepository(db)
	instrRepo := NewDBInstrumenSnapshotRepo(db)
	cpRepo    := NewDBCounterpartyRepo(db)
	kursRepo  := NewDBKursRepository(db)
	ccfRepo   := NewDBCCFConfigRepo(db)

	ccfSvc  := NewCCFLookupService(ccfRepo)
	pdSvc   := NewPDLookupService(pdRepo, instrRepo)
	lgdSvc  := NewLGDLookupService(lgdRepo, instrRepo, cpRepo)
	eadSvc  := NewEADService(instrRepo, kursRepo, ccfSvc)
	bulkSvc := NewBulkHelperService(pdRepo, lgdRepo, instrRepo, cpRepo, kursRepo, ccfRepo, auditWriter, db)

	return &Services{
		PD:        pdSvc,
		LGD:       lgdSvc,
		EAD:       eadSvc,
		CCF:       ccfSvc,
		Bulk:      bulkSvc,
		instrRepo: instrRepo,
	}
}

// previewRepoFromInstrRepo returns the previewInstrumentLister if the instrRepo
// also implements it. Called by NewHandler to wire the preview listing path.
func (s *Services) previewRepoFromInstrRepo() (previewInstrumentLister, bool) {
	if s.instrRepo == nil {
		return nil, false
	}
	p, ok := s.instrRepo.(previewInstrumentLister)
	return p, ok
}
