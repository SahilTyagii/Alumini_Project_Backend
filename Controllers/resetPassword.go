package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	helper "my-go-backend/Helper"
	models "my-go-backend/Models"
	database "my-go-backend/config"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type EmailRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	NewPassWord        string
	token              string
	ConfirmNewPassword string
}

func SendEmail(w http.ResponseWriter, r *http.Request) {
	var emailReq EmailRequest
	err := json.NewDecoder(r.Body).Decode(&emailReq)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	email := emailReq.Email
	fmt.Printf("Received email: %s\n", email)

	// Check if email exists in AlumniProfile
	var alumni models.AlumniProfile
	err = database.DB.Where("email = ?", email).First(&alumni).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Email not found in AlumniProfile", http.StatusNotFound)
			return
		}
		log.Printf("Error searching for email: %v\n", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	token, err := helper.GenerateToken()
	if err != nil {
		log.Printf("Error generating token: %v\n", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	resetLink := helper.GenerateLink(token)
	resetToken := models.ResetPassword{
		Code:      token,
		Email:     email,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	// Check if table exists or create it if it doesn't
	if !database.DB.Migrator().HasTable(&models.ResetPassword{}) {
		if err := database.DB.AutoMigrate(&models.ResetPassword{}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if result := database.DB.Create(&resetToken); result.Error != nil {
		http.Error(w, result.Error.Error(), http.StatusInternalServerError)
		return
	}
	emailBody := fmt.Sprintf(`<p>Click <a href="%s">here</a> to reset your password.</p>`, resetLink)
	err = helper.SendEmail(email, "RESET YOUR PASSWORD", emailBody)
	if err != nil {
		log.Printf("Error sending email: %v\n", err)
		http.Error(w, "Failed to send email", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Email received for reset Password"))
}

func VerifyReset(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return
	}

	// Validate the token
	var resetToken models.ResetPassword
	err := database.DB.Where("code = ?", req.token).First(&resetToken).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			http.Error(w, "Invalid or expired token", http.StatusBadRequest)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check if the token has expired
	if time.Now().After(resetToken.ExpiresAt) {
		http.Error(w, "Token has expired", http.StatusBadRequest)
		return
	}

	if req.NewPassWord != req.ConfirmNewPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}
	// Hash the new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassWord), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	// Find the alumni by email and update the password
	var alumni models.AlumniProfile
	err = database.DB.Where("email = ?", resetToken.Email).First(&alumni).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Alumni not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	alumni.Password = string(hashedPassword)
	err = database.DB.Save(&alumni).Error
	if err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Password has been reset successfully"))
}