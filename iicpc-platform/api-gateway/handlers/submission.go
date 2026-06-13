/*
****** WHAT THIS FILE DOES ******
* Handles all submission related endpoints.
*
* FUNCTIONS:
* - SubmitCode()      => accepts code submission from logged in contestant
*                        validates request, checks submission limits,
*                        saves to Redis and publishes to Kafka
* - GetStatus()       => returns current status of a submission by ID
* - GetSubmissions()  => returns all submissions for logged in user
*
* MULTIPLE SUBMISSIONS SUPPORT:
* Contestants can submit multiple times. Each submission gets a unique ID.
* Previous submissions remain in history with their scores.
* Only the highest score counts for the leaderboard.
* Max 10 active submissions per contestant to prevent abuse.
*
* WHY KAFKA ONLY (no Redis queue)?
* Previously submissions were pushed to both Redis pending-submissions
* list AND Kafka — causing double processing. Now only Kafka is used
* as the submission queue. Redis is only used for storage and sessions.
*/

package handlers

import (
	kafkapkg "api-gateway/kafka"
	"api-gateway/models"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client
var ctx = context.Background()

func InitRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("Redis connection error: %v\n", err)
		return
	}
	fmt.Println("Handlers connected to Redis successfully!")
}

func getSessionUser(c *gin.Context) (string, string, bool) {
	token := c.GetHeader("Authorization")
	if token == "" {
		return "", "", false
	}

	data, err := rdb.Get(ctx, "session:"+token).Result()
	if err != nil {
		return "", "", false
	}

	var session models.Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return "", "", false
	}

	return session.Username, session.Role, true
}

func SubmitCode(c *gin.Context) {
	// get logged in user
	username, _, valid := getSessionUser(c)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "please login first"})
		return
	}

	var req models.SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// check submission limit — max 10 per contestant
	count, err := rdb.LLen(ctx, "submissions:"+username).Result()
	if err == nil && count >= 10 {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error": "maximum 10 submissions allowed per contestant",
		})
		return
	}

	// create submission with unique ID
	submission := models.Submission{
		ID:         fmt.Sprintf("sub-%d-%d", time.Now().Unix(), rand.Intn(1000)),
		Contestant: username,
		Language:   req.Language,
		Code:       req.Code,
		Status:     "queued",
		CreatedAt:  time.Now(),
	}

	// save full submission details to Redis
	data, err := json.Marshal(submission)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process submission"})
		return
	}

	rdb.Set(ctx, "submission:"+submission.ID, data, 24*time.Hour)

	// add to user's submission list — keeps full history
	rdb.LPush(ctx, "submissions:"+username, submission.ID)

	// publish to Kafka only — removed Redis pending-submissions to prevent double processing
	if err := kafkapkg.PublishSubmission(submission); err != nil {
		fmt.Printf("Kafka publish error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue submission"})
		return
	}

	fmt.Printf("Submission queued via Kafka: %s by %s\n", submission.ID, username)

	c.JSON(http.StatusOK, gin.H{
		"submission_id":       submission.ID,
		"status":              "queued",
		"submissions_so_far":  count + 1,
		"message":             "your code has been submitted and is being processed",
	})
}

func GetStatus(c *gin.Context) {
	id := c.Param("id")

	data, err := rdb.Get(ctx, "submission:"+id).Result()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "submission not found"})
		return
	}

	var submission models.Submission
	if err := json.Unmarshal([]byte(data), &submission); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read submission"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"submission_id": submission.ID,
		"contestant":    submission.Contestant,
		"language":      submission.Language,
		"status":        submission.Status,
		"created_at":    submission.CreatedAt,
		"code":          submission.Code,
	})
}

func GetSubmissions(c *gin.Context) {
	username, _, valid := getSessionUser(c)
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "please login first"})
		return
	}

	// get all submission IDs for this user
	ids, err := rdb.LRange(ctx, "submissions:"+username, 0, -1).Result()
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"submissions": []interface{}{}})
		return
	}

	var submissions []models.Submission
	for _, id := range ids {
		id = strings.TrimSpace(id)
		data, err := rdb.Get(ctx, "submission:"+id).Result()
		if err != nil {
			continue
		}

		var sub models.Submission
		if err := json.Unmarshal([]byte(data), &sub); err != nil {
			continue
		}
		submissions = append(submissions, sub)
	}

	c.JSON(http.StatusOK, gin.H{
		"submissions": submissions,
		"total":       len(submissions),
	})
}