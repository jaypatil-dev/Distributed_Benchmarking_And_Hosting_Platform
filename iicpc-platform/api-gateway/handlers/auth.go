/*
****** WHAT THIS FILE DOES ******
* This file handles all authentication endpoints.
*
* FUNCTIONS:
* - Register() => creates a new user account
*   Validates username uniqueness, hashes password,
*   verifies admin secret key, stores in Redis
*
* - Login() => authenticates a user
*   Verifies username exists, checks password hash,
*   generates session token, stores session in Redis
*
* - Logout() => invalidates a session
*   Deletes session token from Redis
*
* - GetMe() => returns current logged in user info
*   Used by frontend to check if session is still valid
*
* WHY SHA-256 FOR PASSWORDS?
* SHA-256 is a one-way hash function. Even if Redis is
* compromised, passwords cannot be reversed. In production
* bcrypt would be used as it is slower and more secure
* against brute force attacks.
*
* WHY RANDOM SESSION TOKENS?
* Each login generates a cryptographically random 32-byte
* token. This prevents session fixation attacks and ensures
* each session is unique and unpredictable.
*/

package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"os"
	"api-gateway/models"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// admin credentials — loaded from environment variables
var adminUsername = getEnv("ADMIN_USERNAME", "admin")
var adminPassword = getEnv("ADMIN_PASSWORD", "iicpc@admin2026")
var adminSecretKey = getEnv("ADMIN_SECRET_KEY", "iicpc@secret2026")

// getEnv reads environment variable with a fallback default
func getEnv(key, fallback string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return fallback
}

var authRdb *redis.Client
var authCtx = context.Background()

func InitAuth(redisClient *redis.Client) {
	authRdb = redisClient

	// create hardcoded admin account if not exists
	exists, _ := authRdb.Exists(authCtx, "user:admin").Result()
	if exists == 0 {
		admin := models.User{
			Username:  adminUsername,
			Password:  hashPassword(adminPassword),
			Role:      "admin",
			CreatedAt: time.Now(),
		}
		data, _ := json.Marshal(admin)
		authRdb.Set(authCtx, "user:admin", data, 0)
		fmt.Println("Admin account created successfully!")
	}
}

func hashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func generateToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func Register(c *gin.Context) {
	var req models.RegisterRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// check if username already exists
	exists, _ := authRdb.Exists(authCtx, "user:"+req.Username).Result()
	if exists > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
		return
	}

	// validate role
	if req.Role != "contestant" && req.Role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	// verify admin secret key
	if req.Role == "admin" {
		if req.SecretKey != adminSecretKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin secret key"})
			return
		}
	}

	// create user
	user := models.User{
		Username:  req.Username,
		Password:  hashPassword(req.Password),
		Role:      req.Role,
		CreatedAt: time.Now(),
	}

	data, _ := json.Marshal(user)
	authRdb.Set(authCtx, "user:"+req.Username, data, 0)

	fmt.Printf("New %s registered: %s\n", req.Role, req.Username)

	c.JSON(http.StatusOK, gin.H{
		"message":  "account created successfully",
		"username": req.Username,
		"role":     req.Role,
	})
}

func Login(c *gin.Context) {
	var req models.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// fetch user from Redis
	data, err := authRdb.Get(authCtx, "user:"+req.Username).Result()
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	var user models.User
	if err := json.Unmarshal([]byte(data), &user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read user"})
		return
	}

	// verify password
	if user.Password != hashPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	// generate session token
	token := generateToken()
	session := models.Session{
		Token:     token,
		Username:  user.Username,
		Role:      user.Role,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	sessionData, _ := json.Marshal(session)
	authRdb.Set(authCtx, "session:"+token, sessionData, 24*time.Hour)

	fmt.Printf("User logged in: %s (%s)\n", user.Username, user.Role)

	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"username": user.Username,
		"role":     user.Role,
		"message":  "login successful",
	})
}

func Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token != "" {
		authRdb.Del(authCtx, "session:"+token)
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

func GetMe(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no token provided"})
		return
	}

	data, err := authRdb.Get(authCtx, "session:"+token).Result()
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
		return
	}

	var session models.Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"username": session.Username,
		"role":     session.Role,
	})
}