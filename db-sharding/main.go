package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type User struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type ShardedDB struct {
	shards [3]*pgx.Conn
}

// connect to all postgres shards
func (s *ShardedDB) ConnectToDB() error {
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		port := 5433 + i

		conn, err := pgx.Connect(
			ctx,
			fmt.Sprintf(
				"postgres://postgres:postgres@localhost:%d/users",
				port,
			),
		)

		if err != nil {
			return fmt.Errorf("failed to connect to shard %d: %w", i, err)
		}

		s.shards[i] = conn

		log.Printf(
			"Connected to shard %d on port %d",
			i,
			port,
		)
	}

	return nil
}

// decide which shard gets the user
// TODO: implement consistant hashing here later
func (s *ShardedDB) GetShard(userId int) (*pgx.Conn, int) {
	shardId := userId % 3

	return s.shards[shardId], shardId
}

// insert the user to correct shard
func (s *ShardedDB) InsertUser(user User) error {
	ctx := context.Background()

	//get the shardid to insert into shard
	db, shardId := s.GetShard(user.Id)

	_, err := db.Exec(
		ctx,
		`INSERT INTO users (id, name, email)
		 VALUES ($1, $2, $3)`,
		user.Id,
		user.Name,
		user.Email,
	)

	if err != nil {
		return err
	}

	log.Printf(
		"User %d inserted into shard %d",
		user.Id,
		shardId,
	)

	return nil
}

// get the user from the correct shard
func (s *ShardedDB) GetUser(userId int) (*User, error) {
	ctx := context.Background()

	db, shardID := s.GetShard(userId)

	var user User

	err := db.QueryRow(
		ctx,
		`SELECT id, name, email
		 FROM users
		 WHERE id = $1`,
		userId,
	).Scan(
		&user.Id,
		&user.Name,
		&user.Email,
	)

	if err != nil {
		return nil, err
	}

	log.Printf("User %d read from shard %d", userId, shardID)

	return &user, nil
}

// POST /users
func (s *ShardedDB) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var user User

	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	err = s.InsertUser(user)
	if err != nil {
		log.Printf("insert failed: %v", err)

		http.Error(w, "failed to insert user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(user)
}

// GET /users/{id}
func (s *ShardedDB) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(
			w,
			"method not allowed",
			http.StatusMethodNotAllowed,
		)
		return
	}

	// /users/123
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	if len(parts) != 2 {
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}

	userID, err := strconv.Atoi(parts[1])
	if err != nil {
		http.Error(w, "invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := s.GetUser(userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}

		log.Printf("query failed: %v", err)

		http.Error(w, "failed to get user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(user)
}

func main() {
	db := &ShardedDB{}

	err := db.ConnectToDB()
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/users", db.CreateUserHandler)

	http.HandleFunc("/users/", db.GetUserHandler)

	log.Println("HTTP server running on :8080")

	err = http.ListenAndServe(":8080", nil)

	if err != nil {
		log.Fatal(err)
	}
}
