/*
****** WHAT THIS FILE DOES ******
* This file handles all admin dashboard endpoints.
* Only users with role "admin" can access these endpoints.
*
* ENDPOINTS:
* - GET /admin/stats      => platform statistics
* - GET /admin/users      => all registered users
* - DELETE /admin/users/:username => delete a user
* - GET /admin/submissions => all submissions from all users
* - POST /admin/trigger/:id => manually trigger test for submission
*
* SECURITY:
* Every endpoint checks the session token and verifies
* the user has admin role before processing the request.
* Non-admin users get 403 Forbidden response.
*/

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"api-gateway/models"

	"github.com/gin-gonic/gin"
)

func isAdmin(c *gin.Context) bool {
	token := c.GetHeader("Authorization")
	if token == "" {
		return false
	}

	data, err := rdb.Get(ctx, "session:"+token).Result()
	if err != nil {
		return false
	}

	var session models.Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return false
	}

	return session.Role == "admin"
}

func GetStats(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	// get all user keys
	userKeys, _ := rdb.Keys(ctx, "user:*").Result()
	totalUsers := 0
	totalContestants := 0
	for _, key := range userKeys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var user models.User
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			continue
		}
		totalUsers++
		if user.Role == "contestant" {
			totalContestants++
		}
	}

	// get all submission keys
	subKeys, _ := rdb.Keys(ctx, "submission:*").Result()
	totalSubmissions := len(subKeys)

	// get leaderboard top scorer
	topScorers, _ := rdb.ZRevRangeWithScores(ctx, "leaderboard", 0, 0).Result()
	topContestant := "none"
	topScore := 0.0
	if len(topScorers) > 0 {
		topContestant = topScorers[0].Member.(string)
		topScore = topScorers[0].Score
	}

	c.JSON(http.StatusOK, gin.H{
		"total_users":       totalUsers,
		"total_contestants": totalContestants,
		"total_admins":      totalUsers - totalContestants,
		"total_submissions": totalSubmissions,
		"top_contestant":    topContestant,
		"top_score":         topScore,
	})
}

func GetAllUsers(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	userKeys, _ := rdb.Keys(ctx, "user:*").Result()
	var users []gin.H

	for _, key := range userKeys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var user models.User
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			continue
		}

		// get submission count for this user
		subCount, _ := rdb.LLen(ctx, "submissions:"+user.Username).Result()

		users = append(users, gin.H{
			"username":         user.Username,
			"role":             user.Role,
			"created_at":       user.CreatedAt,
			"submission_count": subCount,
		})
	}

	if users == nil {
		users = []gin.H{}
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

func DeleteUser(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	username := c.Param("username")

	// prevent deleting admin
	if username == "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete main admin account"})
		return
	}

	// delete user
	rdb.Del(ctx, "user:"+username)

	// delete their submissions list
	rdb.Del(ctx, "submissions:"+username)

	// remove from leaderboard
	rdb.ZRem(ctx, "leaderboard", username)
	rdb.Del(ctx, "contestant:"+username)

	fmt.Printf("Admin deleted user: %s\n", username)

	c.JSON(http.StatusOK, gin.H{
		"message":  "user deleted successfully",
		"username": username,
	})
}

func GetAllSubmissions(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	subKeys, _ := rdb.Keys(ctx, "submission:*").Result()
	var submissions []models.Submission

	for _, key := range subKeys {
		data, err := rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var sub models.Submission
		if err := json.Unmarshal([]byte(data), &sub); err != nil {
			continue
		}
		submissions = append(submissions, sub)
	}

	if submissions == nil {
		submissions = []models.Submission{}
	}

	c.JSON(http.StatusOK, gin.H{"submissions": submissions})
}

func TriggerTest(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	id := c.Param("id")
	id = strings.TrimSpace(id)

	data, err := rdb.Get(ctx, "submission:"+id).Result()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	var sub models.Submission
	if err := json.Unmarshal([]byte(data), &sub); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read submission"})
		return
	}

	// push to pending queue to re-test
	rdb.LPush(ctx, "pending-submissions", id)

	fmt.Printf("Admin triggered re-test for submission: %s\n", id)

	c.JSON(http.StatusOK, gin.H{
		"message":       "test triggered successfully",
		"submission_id": id,
		"contestant":    sub.Contestant,
	})
}