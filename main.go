package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type User struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type UserInput struct {
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Password *string `json:"password"`
}

func main() {
	_ = godotenv.Load(".env")

	ctx := context.Background()
	db, err := ConnectPostgres(ctx)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	if err := ensureUsersTable(ctx, db); err != nil {
		log.Fatalf("failed to ensure users table: %v", err)
	}

	r := gin.Default()

	r.GET("/users", func(c *gin.Context) {
		users, err := listUsers(c.Request.Context(), db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, users)
	})

	r.GET("/users/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}

		user, err := getUser(c.Request.Context(), db, id)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, user)
	})

	r.POST("/users", func(c *gin.Context) {
		var input UserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		user, err := createUser(c.Request.Context(), db, input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, user)
	})

	r.PUT("/users/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}

		var input UserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		user, err := replaceUser(c.Request.Context(), db, id, input)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, user)
	})

	r.PATCH("/users/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}

		var input UserInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		user, err := updateUser(c.Request.Context(), db, id, input)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, user)
	})

	r.DELETE("/users/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
			return
		}

		if err := deleteUser(c.Request.Context(), db, id); err != nil {
			if errors.Is(err, ErrUserNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
	})

	if err := r.Run(":8000"); err != nil {
		log.Fatal(err)
	}
}

var ErrUserNotFound = errors.New("user not found")

func ensureUsersTable(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255),
			email VARCHAR(255),
			phone VARCHAR(20),
			password VARCHAR(255)
		)
	`)
	return err
}

func listUsers(ctx context.Context, db *pgxpool.Pool) ([]User, error) {
	rows, err := db.Query(ctx, `SELECT id, name, email, phone, password FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.Password); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

func getUser(ctx context.Context, db *pgxpool.Pool, id int) (User, error) {
	var user User
	err := db.QueryRow(ctx, `SELECT id, name, email, phone, password FROM users WHERE id=$1`, id).
		Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.Password)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}

func createUser(ctx context.Context, db *pgxpool.Pool, input UserInput) (User, error) {
	var user User
	err := db.QueryRow(
		ctx,
		`INSERT INTO users (name, email, phone, password)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, email, phone, password`,
		derefString(input.Name),
		derefString(input.Email),
		derefString(input.Phone),
		derefString(input.Password),
	).Scan(&user.ID, &user.Name, &user.Email, &user.Phone, &user.Password)
	return user, err
}

func replaceUser(ctx context.Context, db *pgxpool.Pool, id int, input UserInput) (User, error) {
	cmdTag, err := db.Exec(
		ctx,
		`UPDATE users
		 SET name=$1, email=$2, phone=$3, password=$4
		 WHERE id=$5`,
		derefString(input.Name),
		derefString(input.Email),
		derefString(input.Phone),
		derefString(input.Password),
		id,
	)
	if err != nil {
		return User{}, err
	}
	if cmdTag.RowsAffected() == 0 {
		return User{}, ErrUserNotFound
	}
	return getUser(ctx, db, id)
}

func updateUser(ctx context.Context, db *pgxpool.Pool, id int, input UserInput) (User, error) {
	user, err := getUser(ctx, db, id)
	if err != nil {
		return User{}, err
	}

	if input.Name != nil {
		user.Name = *input.Name
	}
	if input.Email != nil {
		user.Email = *input.Email
	}
	if input.Phone != nil {
		user.Phone = *input.Phone
	}
	if input.Password != nil {
		user.Password = *input.Password
	}

	_, err = db.Exec(
		ctx,
		`UPDATE users SET name=$1, email=$2, phone=$3, password=$4 WHERE id=$5`,
		user.Name, user.Email, user.Phone, user.Password, id,
	)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func deleteUser(ctx context.Context, db *pgxpool.Pool, id int) error {
	cmdTag, err := db.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
