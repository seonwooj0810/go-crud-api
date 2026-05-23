package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/seonwoojung/go-crud-api/docs"
)

const (
	responseTimeHeader = "X-Response-Time-Ms"
	defaultDSN         = "postgres://postgres@localhost:5432/postgres?sslmode=disable"
)

type timingWriter struct {
	gin.ResponseWriter
	start time.Time
}

func (w *timingWriter) WriteHeader(code int) {
	ms := float64(time.Since(w.start).Microseconds()) / 1000.0
	w.Header().Set(responseTimeHeader, fmt.Sprintf("%.3f", ms))
	w.ResponseWriter.WriteHeader(code)
}

func ResponseTime() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &timingWriter{ResponseWriter: c.Writer, start: time.Now()}
		c.Next()
	}
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"  example:"정선우"`
	Email string `json:"email" example:"user@example.com"`
}

type UserRequest struct {
	Name  string `json:"name"  binding:"required"       example:"정선우"`
	Email string `json:"email" binding:"required,email" example:"user@example.com"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type UserStore struct {
	pool *pgxpool.Pool
}

func NewUserStore(ctx context.Context, dsn string) (*UserStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &UserStore{pool: pool}
	if err := s.init(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return s, nil
}

func (s *UserStore) Close() { s.pool.Close() }

func (s *UserStore) init(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS users (
            id    TEXT PRIMARY KEY,
            name  TEXT NOT NULL,
            email TEXT NOT NULL UNIQUE
        )
    `)
	return err
}

func (s *UserStore) Create(ctx context.Context, name, email string) (User, error) {
	u := User{ID: uuid.NewString(), Name: name, Email: email}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO users(id, name, email) VALUES($1, $2, $3)`,
		u.ID, u.Name, u.Email,
	)
	return u, err
}

func (s *UserStore) List(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, email FROM users ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *UserStore) Get(ctx context.Context, id string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, email FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Email)
	return u, err
}

func (s *UserStore) Update(ctx context.Context, id, name, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`UPDATE users SET name = $1, email = $2 WHERE id = $3 RETURNING id, name, email`,
		name, email, id,
	).Scan(&u.ID, &u.Name, &u.Email)
	return u, err
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

type Handler struct {
	store *UserStore
}

func (h *Handler) abortWithError(c *gin.Context, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "user not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
}

// CreateUser godoc
// @Summary      유저 생성
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        user  body      UserRequest  true  "유저 정보"
// @Success      201   {object}  User
// @Header       201   {string}  X-Response-Time-Ms  "처리 시간(ms)"
// @Failure      400   {object}  ErrorResponse
// @Failure      500   {object}  ErrorResponse
// @Router       /users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var req UserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	u, err := h.store.Create(c.Request.Context(), req.Name, req.Email)
	if err != nil {
		h.abortWithError(c, err)
		return
	}
	c.JSON(http.StatusCreated, u)
}

// ListUsers godoc
// @Summary      유저 목록
// @Tags         users
// @Produce      json
// @Success      200  {array}   User
// @Header       200  {string}  X-Response-Time-Ms  "처리 시간(ms)"
// @Failure      500  {object}  ErrorResponse
// @Router       /users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.store.List(c.Request.Context())
	if err != nil {
		h.abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, users)
}

// GetUser godoc
// @Summary      유저 단건 조회
// @Tags         users
// @Produce      json
// @Param        id   path      string  true  "유저 ID"
// @Success      200  {object}  User
// @Header       200  {string}  X-Response-Time-Ms  "처리 시간(ms)"
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id} [get]
func (h *Handler) GetUser(c *gin.Context) {
	u, err := h.store.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

// UpdateUser godoc
// @Summary      유저 수정
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id    path      string       true  "유저 ID"
// @Param        user  body      UserRequest  true  "유저 정보"
// @Success      200   {object}  User
// @Header       200   {string}  X-Response-Time-Ms  "처리 시간(ms)"
// @Failure      400   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Failure      500   {object}  ErrorResponse
// @Router       /users/{id} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	var req UserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	u, err := h.store.Update(c.Request.Context(), c.Param("id"), req.Name, req.Email)
	if err != nil {
		h.abortWithError(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

// DeleteUser godoc
// @Summary      유저 삭제
// @Tags         users
// @Param        id   path  string  true  "유저 ID"
// @Success      204
// @Header       204  {string}  X-Response-Time-Ms  "처리 시간(ms)"
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /users/{id} [delete]
func (h *Handler) DeleteUser(c *gin.Context) {
	if err := h.store.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.abortWithError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return defaultDSN
}

// @title       User CRUD API
// @version     1.0
// @description Gin + PostgreSQL 기반 User CRUD 예제. 모든 응답에 X-Response-Time-Ms 헤더로 처리 시간(ms)이 포함됩니다.
// @host        localhost:8080
// @BasePath    /api/v1
func main() {
	ctx := context.Background()
	store, err := NewUserStore(ctx, dsn())
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
	}
	defer store.Close()

	h := &Handler{store: store}
	r := gin.Default()
	r.Use(ResponseTime())

	api := r.Group("/api/v1")
	{
		api.POST("/users", h.CreateUser)
		api.GET("/users", h.ListUsers)
		api.GET("/users/:id", h.GetUser)
		api.PUT("/users/:id", h.UpdateUser)
		api.DELETE("/users/:id", h.DeleteUser)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server: %v", err)
	}
}
