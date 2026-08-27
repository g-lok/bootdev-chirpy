package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/g-lok/bootdev-chirpy/internal/database"
	_ "github.com/lib/pq"
	"github.com/lmittmann/tint"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, req)
	})
}

func censorWord(text string) string {
	words := []string{"kerfuffle", "fornax", "sharbert"}

	wordSet := make(map[string]bool, len(words))
	for _, w := range words {
		wordSet[strings.ToLower(w)] = true
	}

	runes := []rune(text)
	var sb strings.Builder
	i := 0
	for i < len(runes) {
		if !unicode.IsLetter(runes[i]) && !unicode.IsDigit(runes[i]) {
			sb.WriteRune(runes[i])
			i++
			continue
		}

		start := i
		for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i])) {
			i++
		}
		word := string(runes[start:i])
		followedByPunct := i < len(runes) && unicode.IsPunct(runes[i])

		if !followedByPunct && wordSet[strings.ToLower(word)] {
			sb.WriteString("****")
			continue
		}
		sb.WriteString(word)
	}
	return sb.String()
}

func (cfg *apiConfig) getVisits(w http.ResponseWriter, req *http.Request) {
	hitsHTML := `
	<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %v times!</p>
  </body>
</html>
	`
	hits := fmt.Sprintf(hitsHTML, cfg.fileserverHits.Load())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(hits))
}

func (cfg *apiConfig) resetVisits(w http.ResponseWriter, req *http.Request) {
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Visits counter reset"))
}

func healthz(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func validateChirps(w http.ResponseWriter, req *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	type errJson struct {
		Error string `json:"error"`
	}

	type validJson struct {
		CensoredSpeech string `json:"cleaned_body"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		slog.Error("error decoding parameters", "error", err)
		respBody := errJson{
			Error: fmt.Sprintf("Error decoding parameters: %s", err),
		}

		data, err := json.Marshal(respBody)
		if err != nil {
			slog.Error("error marshalling data", "error", err)
			w.WriteHeader(500)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write(data)
		return
	}

	if len(params.Body) > 140 {
		respBody := errJson{
			Error: "Cannot accept Chirps > 140 chars",
		}

		data, err := json.Marshal(respBody)
		if err != nil {
			slog.Error("error marshalling data", "error", err)
			w.WriteHeader(500)
			w.Header().Set("Content-Type", "application/json")
			w.Write(data)
			return
		}

		w.WriteHeader(400)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
		return
	}

	respBody := validJson{
		CensoredSpeech: censorWord(params.Body),
	}
	data, err := json.Marshal(respBody)
	if err != nil {
		slog.Error("error marshalling data", "error", err)

		w.WriteHeader(500)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write(data)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

func main() {
	w := os.Stderr
	logger := slog.New(tint.NewTextHandler(w, nil))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		logger.Error("failed to open db", "error", err, "table", "chirpy")
		os.Exit(1)
	}

	dbQueries := database.New(db)

	var apiCfg apiConfig
	apiCfg.dbQueries = dbQueries

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	fsClean := http.StripPrefix("/app/", fs)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fsClean))
	mux.HandleFunc("GET /admin/metrics", apiCfg.getVisits)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetVisits)
	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/validate_chirp", validateChirps)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	logger.Info("Starting chirpy server", "port", 8080)
	err = server.ListenAndServe()
	if err != nil {
		logger.Error("failed to start server", "error", err)
	}
}
