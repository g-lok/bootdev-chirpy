package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/g-lok/bootdev-chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/lmittmann/tint"
)

const maxChirpLen int = 140

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
}

func sendJSON[T any](code int, w http.ResponseWriter, payload T) {
	data, err := json.Marshal(payload)
	if err != nil {
		apiError("error marshalling data", "Error encoding JSON", err, 500, w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, req)
	})
}

func simpleError(errMsg string, code int, w http.ResponseWriter) {
	slog.Error(errMsg)
	w.WriteHeader(code)
}

func apiError(logMsg string, apiMsg string, err error, code int, w http.ResponseWriter) {
	type errJSON struct {
		Error string `json:"error"`
	}

	slog.Error(logMsg, "error", err)
	respBody := errJSON{
		Error: fmt.Sprintf(apiMsg, err),
	}

	sendJSON(code, w, respBody)
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

func (cfg *apiConfig) reset(w http.ResponseWriter, req *http.Request) {
	if cfg.platform != "dev" {
		simpleError("forbidden: platform env is not dev", 403, w)
		return
	}

	ctx := req.Context()

	err := cfg.db.DeleteUsers(ctx)
	if err != nil {
		apiMsg := "Error dropping all users from db"
		apiError("error deleting all uesrs", apiMsg, err, 500, w)
		return
	}

	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Visits counter and users table reset"))
}

func healthz(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) postUsers(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type parameters struct {
		Email string `json:"email"`
	}

	type okJSON struct {
		Id        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email     string `json:"email"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		apiError("error decoding parameters", "Error decoding JSON parameter", err, http.StatusInternalServerError, w)
		return
	}

	userExists, err := db.UserExists(ctx, params.Email)
	if err != nil {
		apiError("error checking if user exists", "Error checking user table", err, http.StatusInternalServerError, w)
		return
	}

	if userExists {
		apiError("user with email already exists", "User alredy exists", err, http.StatusInternalServerError, w)
		return
	}

	usr, err := db.CreateUser(ctx, params.Email)
	if err != nil {
		apiError("error adding new user", "Error adding new user", err, http.StatusInternalServerError, w)
		return
	}

	respBody := okJSON{
		Id:        usr.ID.String(),
		CreatedAt: usr.CreatedAt.String(),
		UpdatedAt: usr.UpdatedAt.String(),
		Email:     usr.Email,
	}

	sendJSON(http.StatusCreated, w, respBody)
}

func (cfg *apiConfig) postChirps(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type parameters struct {
		Body   string `json:"body"`
		UserID string `json:"user_id"`
	}

	type okJSON struct {
		Id        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body      string `json:"body"`
		UserID    string `json:"user_id"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		apiError("error decoding parameters", "Error decoding JSON", err, http.StatusInternalServerError, w)
		return
	}

	userID, err := uuid.Parse(params.UserID)
	if err != nil {
		apiError("invalid user UUID", "Invalid user_id: not a UUID", err, http.StatusBadRequest, w)
		return
	}

	if len(params.Body) > maxChirpLen {
		err := errors.New("cannot create chirp > 140 chars")
		apiError("chirp > 140 chars", "Cannot create chirps > 140chars", err, http.StatusUnprocessableEntity, w)
		return
	}

	params.Body = censorWord(params.Body)

	chirpParams := database.CreateChirpParams{
		Body:   params.Body,
		UserID: userID,
	}

	chirp, err := db.CreateChirp(ctx, chirpParams)
	if err != nil {
		apiError("error creating new chirp", "Error adding new chirp", err, http.StatusInternalServerError, w)
		return
	}

	respBody := okJSON{
		Id:        chirp.ID.String(),
		CreatedAt: chirp.CreatedAt.String(),
		UpdatedAt: chirp.UpdatedAt.String(),
		Body:      chirp.Body,
		UserID:    chirp.UserID.String(),
	}

	sendJSON(http.StatusCreated, w, respBody)
}

func main() {
	w := os.Stderr
	logger := slog.New(tint.NewTextHandler(w, nil))
	slog.SetDefault(logger)

	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		logger.Error("failed to open db", "error", err, "table", "chirpy")
		os.Exit(1)
	}

	dbQueries := database.New(db)

	var apiCfg apiConfig
	apiCfg.db = dbQueries
	apiCfg.platform = os.Getenv("PLATFORM")

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	fsClean := http.StripPrefix("/app/", fs)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fsClean))
	mux.HandleFunc("GET /admin/metrics", apiCfg.getVisits)
	mux.HandleFunc("POST /admin/reset", apiCfg.reset)
	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/users", apiCfg.postUsers)
	mux.HandleFunc("POST /api/chirps", apiCfg.postChirps)

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
