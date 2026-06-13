/*
****** WHAT THIS FILE DOES ******
* This file defines the data structures for user authentication.
*
* STRUCTURES:
* - User => represents a registered user (contestant or admin)
*   Contains username, hashed password, role and registration time.
*
* - Session => represents an active login session.
*   Contains session token, username, role and expiry time.
*
* - RegisterRequest => JSON body expected when registering.
*   Contains username, password, role and optional secret key for admin.
*
* - LoginRequest => JSON body expected when logging in.
*   Contains username and password.
*
* ROLES:
* - "contestant" => can submit code, view own submissions, view leaderboard
* - "admin" => can view all submissions, manage platform
*
* WHY HASH PASSWORDS?
* Never store plain text passwords. We use SHA-256 hashing so even if
* Redis is compromised, passwords cannot be recovered.
*
* SESSION TOKENS:
* After login a unique session token is generated and stored in Redis
* with 24 hour expiry. The browser stores this token in localStorage
* and sends it with every request for authentication.
*/

package models

import "time"

// User represents a registered user
type User struct {
	Username  string    `json:"username"`
	Password  string    `json:"password"` // stored as SHA-256 hash
	Role      string    `json:"role"`     // "contestant" or "admin"
	CreatedAt time.Time `json:"created_at"`
}

// Session represents an active login session
type Session struct {
	Token     string    `json:"token"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RegisterRequest is the JSON body for registration
type RegisterRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	Role      string `json:"role" binding:"required"` // "contestant" or "admin"
	SecretKey string `json:"secret_key"`              // required only for admin
}

// LoginRequest is the JSON body for login
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}