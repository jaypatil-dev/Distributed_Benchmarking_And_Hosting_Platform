/*
Purpose: This file defines the data models for submissions in the coding contest application. It includes the Submission struct, which represents a submission made by a contestant, and the SubmitRequest struct, which is used to capture the data required to create a new submission.
* Submission is the full object we store — has an ID, contestant name, their code, status etc.
* SubmitRequest is what the contestant sends us — just 3 fields.
* binding:"required" means Gin will automatically reject requests missing these fields.
* json:"id" tells Go — when converting to JSON, call this field id not ID.
* 

*/


package models

import "time"

type Submission struct {
	ID         string    `json:"id"`
	Contestant string    `json:"contestant"`
	Language   string    `json:"language"`
	Code       string    `json:"code"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type SubmitRequest struct {
	Contestant string `json:"contestant" binding:"required"`
	Language   string `json:"language" binding:"required"`
	Code       string `json:"code" binding:"required"`
}