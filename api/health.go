package api

import (
	"net/http"
	"time"

	"github.com/dimiro1/health"
	"github.com/hermeznetwork/hermez-node/health/checkers"
)

func (a *API) healthRoute(version string) http.Handler {
	// taking two checkers for one db in case that in
	// the future there will be two separated dbs
	l2DBChecker := checkers.NewCheckerWithDB(a.l2.DB().DB)
	historyDBChecker := checkers.NewCheckerWithDB(a.h.DB().DB)
	healthHandler := health.NewHandler()
	healthHandler.AddChecker("l2DB", l2DBChecker)
	healthHandler.AddChecker("historyDB", historyDBChecker)
	healthHandler.AddInfo("version", version)
	t := time.Now().UTC()
	healthHandler.AddInfo("timestamp", t)
	return healthHandler
}
