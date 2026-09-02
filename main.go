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
	"time"
	"unicode"

	"github.com/g-lok/bootdev-chirpy/internal/auth"
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
	secret         string
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
		Error: fmt.Sprintf("%s: %v", apiMsg, err),
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
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type okJSON struct {
		Id          string `json:"id"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
		Email       string `json:"email"`
		IsChirpyRed bool   `json:"is_chirpy_red"`
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

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		apiError("failed to hash password", "Error processing password", err, http.StatusInternalServerError, w)
		return
	}

	newUserParams := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}

	usr, err := db.CreateUser(ctx, newUserParams)
	if err != nil {
		apiError("error adding new user", "Error adding new user", err, http.StatusInternalServerError, w)
		return
	}

	respBody := okJSON{
		Id:          usr.ID.String(),
		CreatedAt:   usr.CreatedAt.String(),
		UpdatedAt:   usr.UpdatedAt.String(),
		Email:       usr.Email,
		IsChirpyRed: usr.IsChirpyRed.Bool,
	}

	sendJSON(http.StatusCreated, w, respBody)
}

func (cfg *apiConfig) putUsers(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type okJSON struct {
		ID          string `json:"id"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
		Email       string `json:"email"`
		IsChirpyRed bool   `json:"is_chirpy_red"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		apiError("error decoding parameters", "Error decoding JSON parameter", err, http.StatusInternalServerError, w)
		return
	}

	bearerToken, err := auth.GetBearerToken(req.Header)
	if err != nil {
		apiError("failed to get bearer token", "Failed to get bearer token", err, http.StatusUnauthorized, w)
		return
	}

	userID, err := auth.ValidateJWT(bearerToken, cfg.secret)
	if err != nil {
		apiError("failed to authorize bearer token", "Failed authentication", err, http.StatusUnauthorized, w)
		return
	}

	hashedPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		apiError("failed to hash password", "Error processing password", err, http.StatusInternalServerError, w)
		return
	}

	putUserParams := database.PutUserParams{
		ID:             userID,
		Email:          params.Email,
		HashedPassword: hashedPassword,
	}
	updatedUsr, err := db.PutUser(ctx, putUserParams)
	if err != nil {
		apiError("failed to update user", "Failed to update user", err, http.StatusInternalServerError, w)
	}

	respBody := okJSON{
		ID:          updatedUsr.ID.String(),
		Email:       updatedUsr.Email,
		CreatedAt:   updatedUsr.CreatedAt.String(),
		UpdatedAt:   updatedUsr.UpdatedAt.String(),
		IsChirpyRed: updatedUsr.IsChirpyRed.Bool,
	}

	sendJSON(http.StatusOK, w, respBody)
}

func (cfg *apiConfig) postPolkaWebhooks(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type dataParameter struct {
		UserID string `json:"user_id"`
	}

	type parameters struct {
		Event string        `json:"event"`
		Data  dataParameter `json:"data"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		apiError("error decoding parameters", "Error decoding JSON parameter", err, http.StatusInternalServerError, w)
		return
	}

	if params.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	userID, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = db.UpdateUserRed(ctx, userID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) postLogin(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	type okJSON struct {
		ID           string `json:"id"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		Email        string `json:"email"`
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
		IsChirpyRed  bool   `json:"is_chirpy_red"`
	}

	decoder := json.NewDecoder(req.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		apiError("error decoding parameters", "Error decoding JSON parameter", err, http.StatusInternalServerError, w)
		return
	}

	refreshTokenExpirationDuration := time.Hour * 24 * 60
	jwtExpiration := 3600
	jwtExpirationDuration := time.Second * time.Duration(jwtExpiration)

	usr, err := db.GetUserByEmail(ctx, params.Email)
	if err != nil {
		apiError("failed to get user", "Failed to get user", err, http.StatusNotFound, w)
		return
	}

	checkPassword, err := auth.CheckHashedPassword(params.Password, usr.HashedPassword)
	if err != nil {
		apiError("failed to check password", "Failed to check password", err, http.StatusInternalServerError, w)
		return
	}

	if checkPassword {
		token, err := auth.MakeJWT(usr.ID, cfg.secret, jwtExpirationDuration)
		if err != nil {
			apiError("failed to create jwt token", "Failed to generate JWT token", err, http.StatusInternalServerError, w)
			return
		}

		userID, err := auth.ValidateJWT(token, cfg.secret)
		if err != nil {
			apiError("failed JWT validation", "Failed JWT validation", err, http.StatusUnauthorized, w)
			return
		}

		refreshToken := auth.MakeRefreshToken()
		refreshTokenParams := database.CreateRefreshTokenParams{
			Token:     refreshToken,
			UserID:    userID,
			ExpiresAt: time.Now().Add(refreshTokenExpirationDuration),
		}
		_, err = db.CreateRefreshToken(ctx, refreshTokenParams)
		if err != nil {
			apiError("failed to generate refresh token", "Failed to create refresh token", err, http.StatusInternalServerError, w)
			return
		}

		respBody := okJSON{
			ID:           userID.String(),
			CreatedAt:    usr.CreatedAt.String(),
			UpdatedAt:    usr.UpdatedAt.String(),
			Email:        usr.Email,
			Token:        token,
			RefreshToken: refreshToken,
			IsChirpyRed:  usr.IsChirpyRed.Bool,
		}

		sendJSON(http.StatusOK, w, respBody)
	} else {
		err = errors.New("failed authentication")
		apiError("authentication failed", "Authentication failed", err, http.StatusUnauthorized, w)
	}
}

func (cfg *apiConfig) checkRefreshToken(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type okJSON struct {
		Token string `json:"token"`
	}

	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		errMsg := errors.New("no refresh token found in header")
		apiError("refhresh token Authorization header not found", "Authorization header not found", errMsg, http.StatusBadRequest, w)
		return
	}

	bearer := strings.TrimPrefix(authHeader, "Bearer ")
	rToken, err := db.GetRefreshToken(ctx, bearer)
	if err != nil {
		apiError("error retrieving refresh token", "Error retrieving refresh token", err, http.StatusUnauthorized, w)
		return
	}

	if rToken.ExpiresAt.Before(time.Now()) || rToken.RevokedAt.Valid {
		errMsg := errors.New("refresh token expired/revoked")
		apiError("token expired/revoked", "Invliad refresh token", errMsg, http.StatusUnauthorized, w)
		return
	}

	token, err := auth.MakeJWT(rToken.UserID, cfg.secret, time.Hour)
	if err != nil {
		apiError("failed to create jwt token", "Failed to generate JWT token", err, http.StatusInternalServerError, w)
		return
	}

	respBody := okJSON{
		Token: token,
	}

	sendJSON(http.StatusOK, w, respBody)
}

func (cfg *apiConfig) revokeRefreshToken(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		errMsg := errors.New("no refresh token found in header")
		apiError("refhresh token Authorization header not found", "Authorization header not found", errMsg, http.StatusBadRequest, w)
		return
	}

	bearer := strings.TrimPrefix(authHeader, "Bearer ")
	err := db.RevokeRefreshToken(ctx, bearer)
	if err != nil {
		apiError("error revoking refresh token", "Error revoking refresh token", err, http.StatusBadRequest, w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) postChirps(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type parameters struct {
		Body string `json:"body"`
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

	bearerToken, err := auth.GetBearerToken(req.Header)
	if err != nil {
		apiError("failed to get bearer token", "Failed to get bearer token", err, http.StatusInternalServerError, w)
		return
	}

	userID, err := auth.ValidateJWT(bearerToken, cfg.secret)
	if err != nil {
		apiError("failed to authorize bearer token", "Failed authentication", err, http.StatusUnauthorized, w)
		return
	}

	if len(params.Body) > maxChirpLen {
		errMsg := errors.New("cannot create chirp > 140 chars")
		apiError("chirp > 140 chars", "Cannot create chirps > 140chars", errMsg, http.StatusUnprocessableEntity, w)
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

func (cfg *apiConfig) getChirps(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type chirpJSON struct {
		Id        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body      string `json:"body"`
		UserID    string `json:"user_id"`
	}

	type chirpsList struct {
		Data []chirpJSON `json:"data"`
	}

	chirps, err := db.GetChirps(ctx)
	if err != nil {
		apiError("error fetching chirps", "Error getting chirps", err, http.StatusInternalServerError, w)
		return
	}

	var respJSON chirpsList

	for _, chirp := range chirps {
		chirpFormatted := chirpJSON{
			Id:        chirp.ID.String(),
			CreatedAt: chirp.CreatedAt.String(),
			UpdatedAt: chirp.UpdatedAt.String(),
			Body:      chirp.Body,
			UserID:    chirp.UserID.String(),
		}

		respJSON.Data = append(respJSON.Data, chirpFormatted)
	}

	sendJSON(http.StatusOK, w, respJSON.Data)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type chirpJSON struct {
		Id        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body      string `json:"body"`
		UserID    string `json:"user_id"`
	}

	chirpID, err := uuid.Parse(req.PathValue("chirpID"))
	if err != nil {
		apiError("invlaid chirpID uuid", "Invalid chirpID uuid", err, http.StatusBadRequest, w)
		return
	}

	chirp, err := db.GetChirp(ctx, chirpID)
	if err != nil {
		apiError("error fetching chirp", "Error getting chirp", err, http.StatusNotFound, w)
		return
	}

	respJSON := chirpJSON{
		Id:        chirp.ID.String(),
		CreatedAt: chirp.CreatedAt.String(),
		UpdatedAt: chirp.UpdatedAt.String(),
		Body:      chirp.Body,
		UserID:    chirp.UserID.String(),
	}

	sendJSON(http.StatusOK, w, respJSON)
}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	db := cfg.db

	type chirpJSON struct {
		Id        string `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body      string `json:"body"`
		UserID    string `json:"user_id"`
	}

	bearerToken, err := auth.GetBearerToken(req.Header)
	if err != nil {
		apiError("failed to get bearer token", "Failed to get bearer token", err, http.StatusUnauthorized, w)
		return
	}

	userID, err := auth.ValidateJWT(bearerToken, cfg.secret)
	if err != nil {
		apiError("failed to authorize bearer token", "Failed authentication", err, http.StatusUnauthorized, w)
		return
	}

	chirpID, err := uuid.Parse(req.PathValue("chirpID"))
	if err != nil {
		apiError("invlaid chirpID uuid", "Invalid chirpID uuid", err, http.StatusBadRequest, w)
		return
	}

	chirp, err := db.GetChirp(ctx, chirpID)
	if err != nil {
		apiError("error fetching chirp", "Error getting chirp", err, http.StatusNotFound, w)
		return
	}

	if chirp.UserID != userID {
		errMsg := errors.New("user not authorized to delete chirp")
		apiError("user not authorized to delete chirp", "User not authorized to delete chirp", errMsg, http.StatusForbidden, w)
		return
	}

	err = db.DeleteChirp(ctx, chirpID)
	if err != nil {
		apiError("failed to delete chirp", "Failed to delete chirp", err, http.StatusInternalServerError, w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
	apiCfg.secret = os.Getenv("SECRET")

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	fsClean := http.StripPrefix("/app/", fs)
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(fsClean))
	mux.HandleFunc("GET /admin/metrics", apiCfg.getVisits)
	mux.HandleFunc("POST /admin/reset", apiCfg.reset)
	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/users", apiCfg.postUsers)
	mux.HandleFunc("PUT /api/users", apiCfg.putUsers)
	mux.HandleFunc("POST /api/login", apiCfg.postLogin)
	mux.HandleFunc("POST /api/refresh", apiCfg.checkRefreshToken)
	mux.HandleFunc("POST /api/revoke", apiCfg.revokeRefreshToken)
	mux.HandleFunc("POST /api/chirps", apiCfg.postChirps)
	mux.HandleFunc("GET /api/chirps", apiCfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteChirp)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.postPolkaWebhooks)

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
