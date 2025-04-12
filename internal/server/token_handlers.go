package server

import (
	"harvest/internal/controller"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// TokenNameRequest represents the request for creating a token with a custom name
type TokenNameRequest struct {
	Name string `json:"name" binding:"required"`
}

// TokenResponse represents the response for token operations
type TokenResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	LastUsed  time.Time  `json:"lastUsed"`
	Revoked   bool       `json:"revoked"`
}

// TokenWithStringResponse includes the actual token string for creation operations
type TokenWithStringResponse struct {
	Token string        `json:"token"`
	Info  TokenResponse `json:"info"`
}

// CreateTokenHandler creates a new service token with the provided name
func (s *Server) CreateTokenHandler(c *gin.Context) {
	var req TokenNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use the provided name for the token
	tokenName := req.Name

	// Generate the token with SERVICE role and no expiration
	tokenString, token, err := s.tc.GenerateToken(c.Request.Context(), tokenName, controller.RoleService, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token: " + err.Error()})
		return
	}

	// Prepare response
	var expiresAt *time.Time
	if !token.ExpiresAt.IsZero() {
		expiresAt = &token.ExpiresAt
	}

	response := TokenWithStringResponse{
		Token: tokenString,
		Info: TokenResponse{
			ID:        token.ID.Hex(),
			Name:      token.Name,
			Role:      token.Role,
			CreatedAt: token.CreatedAt,
			ExpiresAt: expiresAt,
			LastUsed:  token.LastUsed,
			Revoked:   token.Revoked,
		},
	}

	c.JSON(http.StatusCreated, response)
}

// ListTokensHandler returns a list of all tokens
func (s *Server) ListTokensHandler(c *gin.Context) {
	tokens, err := s.tc.ListTokens(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list tokens: " + err.Error()})
		return
	}

	// Convert to response format
	var response []TokenResponse
	for _, token := range tokens {
		var expiresAt *time.Time
		if !token.ExpiresAt.IsZero() {
			expiresAt = &token.ExpiresAt
		}

		tokenResp := TokenResponse{
			ID:        token.ID.Hex(),
			Name:      token.Name,
			Role:      token.Role,
			CreatedAt: token.CreatedAt,
			ExpiresAt: expiresAt,
			LastUsed:  token.LastUsed,
			Revoked:   token.Revoked,
		}

		response = append(response, tokenResp)
	}

	c.JSON(http.StatusOK, response)
}

// GetTokenHandler returns a specific token by ID
func (s *Server) GetTokenHandler(c *gin.Context) {
	tokenID := c.Param("id")

	token, err := s.tc.GetTokenByID(c.Request.Context(), tokenID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get token: " + err.Error()})
		return
	}

	if token == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Token not found"})
		return
	}

	// Prepare response
	var expiresAt *time.Time
	if !token.ExpiresAt.IsZero() {
		expiresAt = &token.ExpiresAt
	}

	response := TokenResponse{
		ID:        token.ID.Hex(),
		Name:      token.Name,
		Role:      token.Role,
		CreatedAt: token.CreatedAt,
		ExpiresAt: expiresAt,
		LastUsed:  token.LastUsed,
		Revoked:   token.Revoked,
	}

	c.JSON(http.StatusOK, response)
}

// RevokeTokenHandler revokes a token
func (s *Server) RevokeTokenHandler(c *gin.Context) {
	tokenID := c.Param("id")

	err := s.tc.RevokeToken(c.Request.Context(), tokenID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token revoked successfully"})
}
