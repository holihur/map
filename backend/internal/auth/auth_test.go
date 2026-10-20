package auth

import (
	"testing"
	"time"

	"mapserver/internal/models"
)

func TestSignAndParse(t *testing.T) {
	secret := []byte("test-secret")
	user := &models.User{ID: 7, Username: "alice", Role: RoleEditor}

	token, err := Sign(secret, user, time.Hour)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := Parse(secret, token)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != 7 || claims.Username != "alice" || claims.Role != RoleEditor {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	if _, err := Parse([]byte("wrong-secret"), token); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestExpiredToken(t *testing.T) {
	token, _ := Sign([]byte("s"), &models.User{ID: 1, Role: RoleViewer}, -time.Minute)
	if _, err := Parse([]byte("s"), token); err == nil {
		t.Fatal("expected expired token error")
	}
}

func TestPasswordHash(t *testing.T) {
	hash, err := HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !CheckPassword(hash, "secret123") {
		t.Fatal("expected password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected password mismatch")
	}
}

func TestCan(t *testing.T) {
	cases := []struct {
		role, need string
		want       bool
	}{
		{RoleAdmin, RoleEditor, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleEditor, RoleEditor, true},
		{RoleEditor, RoleAdmin, false},
		{RoleViewer, RoleEditor, false},
		{RoleViewer, RoleViewer, true},
	}
	for _, c := range cases {
		if got := Can(c.role, c.need); got != c.want {
			t.Errorf("Can(%s,%s)=%v want %v", c.role, c.need, got, c.want)
		}
	}
}
