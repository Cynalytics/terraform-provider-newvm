package newvm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Login - Get a new token for user
func (c *Client) Login(ctx context.Context) (*AuthResponse, error) {
	if c.Auth.Username == "" || c.Auth.Password == "" {
		return nil, fmt.Errorf("define username and password")
	}
	rb, err := json.Marshal(c.Auth)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/identity/v1", c.HostURL), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	ar := AuthResponse{}
	err = json.Unmarshal(body, &ar)
	if err != nil {
		return nil, err
	}

	return &ar, nil
}

// Logout - Revoke the token for a user
func (c *Client) Logout(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("%s/identity/v1", c.HostURL), strings.NewReader(string("")))
	if err != nil {
		return err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return err
	}

	if strings.ReplaceAll(string(body), " ", "") != "{\"success\":true}" {
		return errors.New(string(body))
	}

	return nil
}
