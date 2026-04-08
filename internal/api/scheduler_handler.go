package api

import (
	"encoding/json"
	"net/http"
)

// handleSchedulerStatus returns the scheduler's current state.
func (s *Server) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"status":  map[string]interface{}{"enabled": false},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"status":  s.scheduler.Status(),
	})
}

// handleSchedulerRun triggers a manual scheduler run.
func (s *Server) handleSchedulerRun(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Scheduler not initialized",
		})
		return
	}
	go s.scheduler.Run()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Scheduler run triggered",
	})
}

// handleSchedulerConfig updates scheduler configuration.
func (s *Server) handleSchedulerConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled       *bool `json:"enabled"`
		IntervalHours *int  `json:"interval_hours"`
		AutoDownload  *bool `json:"auto_download"`
		MinScore      *int  `json:"min_score"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Invalid JSON",
		})
		return
	}

	if req.Enabled != nil {
		s.cfg.SchedulerEnabled = *req.Enabled
	}
	if req.IntervalHours != nil && *req.IntervalHours >= 1 {
		s.cfg.SchedulerIntervalHours = *req.IntervalHours
	}
	if req.AutoDownload != nil {
		s.cfg.SchedulerAutoDownload = *req.AutoDownload
	}
	if req.MinScore != nil && *req.MinScore >= 0 && *req.MinScore <= 100 {
		s.cfg.SchedulerMinScore = *req.MinScore
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"config": map[string]interface{}{
			"enabled":        s.cfg.SchedulerEnabled,
			"interval_hours": s.cfg.SchedulerIntervalHours,
			"auto_download":  s.cfg.SchedulerAutoDownload,
			"min_score":      s.cfg.SchedulerMinScore,
		},
	})
}

// handleListSeries returns detected series with completion status.
func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	if s.seriesDetector == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"series":  []interface{}{},
		})
		return
	}

	series, err := s.seriesDetector.DetectSeries()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false, "error": "Failed to detect series",
		})
		return
	}
	if series == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"series":  []interface{}{},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"series":  series,
	})
}

// handleSeriesMissing returns missing books for a specific series.
func (s *Server) handleSeriesMissing(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Series name is required",
		})
		return
	}

	if s.seriesDetector == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"missing": []string{},
		})
		return
	}

	missing, err := s.seriesDetector.GetMissing(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"success": false, "error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"series":  name,
		"missing": missing,
	})
}

// handleSearchMissingSeries searches for missing books in a series.
func (s *Server) handleSearchMissingSeries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "Series name is required",
		})
		return
	}

	if s.seriesDetector == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"results": []interface{}{},
		})
		return
	}

	results, err := s.seriesDetector.SearchMissing(name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{
			"success": false, "error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"series":  name,
		"results": results,
	})
}
