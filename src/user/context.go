package user

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/crypt"
)

// Context for authenticated users
type Context struct {
	context.Context
	ID        string    `json:"id"`
	Anonymous bool      `json:"an"`
	LastLogin time.Time `json:"ll"`
}

// MarshalJSON returns the user context as valid, encrypted JSON
func (c *Context) MarshalJSON() ([]byte, error) {
	data := struct {
		ID        string    `json:"id"`
		Anonymous bool      `json:"an"`
		LastLogin time.Time `json:"ll"`
	}{
		ID:        c.ID,
		Anonymous: c.Anonymous,
		LastLogin: c.LastLogin,
	}

	// marshal and encrypt the context data
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	encryptedJson, err := crypt.Encrypt(jsonData)
	if err != nil {
		return nil, err
	}

	return encryptedJson, nil
}

// GetID is a helper to return the user ID from the request context
func GetID(ctx *fiber.Ctx) string {
	c := GetContext(ctx)
	if c == nil {
		return ""
	}
	return c.ID
}

// GetContext returns the decrypted Context from within the fiber.Context
func GetContext(ctx *fiber.Ctx) *Context {
	c, ok := ctx.UserContext().(*Context)
	if ok {
		return c
	}

	return nil
}

// Anonymous returns a new anonymous user context
func Anonymous() *Context {
	return &Context{
		Context:   context.Background(),
		ID:        config.GenerateCode(16, config.Base58),
		Anonymous: true,
		LastLogin: time.Now(),
	}
}
