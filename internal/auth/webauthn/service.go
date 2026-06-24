package webauthn

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type User struct {
	ID          string
	Username    string
	DisplayName string
	Credentials []webauthn.Credential
}

func (u User) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u User) WebAuthnName() string                       { return u.Username }
func (u User) WebAuthnDisplayName() string                { return u.DisplayName }
func (u User) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

func NewService(rpID, origin string) (*webauthn.WebAuthn, error) {
	if rpID == "" {
		rpID = "localhost"
	}
	if origin == "" {
		origin = "http://localhost:8080"
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "DataSafeS3",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

func RPFromRequest(r *http.Request) (rpID, origin string) {
	host := r.Host
	if i := strings.Index(host, ":"); i > 0 {
		host = host[:i]
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	return host, fmt.Sprintf("%s://%s", scheme, r.Host)
}

func ParseCredentials(raw string) []webauthn.Credential {
	if raw == "" {
		return nil
	}
	var creds []webauthn.Credential
	_ = json.Unmarshal([]byte(raw), &creds)
	return creds
}

func MarshalCredentials(creds []webauthn.Credential) (string, error) {
	if len(creds) == 0 {
		return "", nil
	}
	data, err := json.Marshal(creds)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FinishRegistration(w *webauthn.WebAuthn, user User, session webauthn.SessionData, r *http.Request) (webauthn.Credential, error) {
	cred, err := w.FinishRegistration(user, session, r)
	if err != nil {
		return webauthn.Credential{}, err
	}
	return *cred, nil
}

func BeginLogin(w *webauthn.WebAuthn, user User) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	return w.BeginLogin(user)
}

func FinishLogin(w *webauthn.WebAuthn, user User, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	return w.FinishLogin(user, session, r)
}
