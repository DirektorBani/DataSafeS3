package api

import (
	"net/http"

	"github.com/DirektorBani/datasafe/internal/security/urlpolicy"
)

func validateOutboundURL(raw string) error {
	return urlpolicy.ValidateOutboundURL(raw, urlpolicy.DefaultOptions())
}

func writeOutboundURLError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": urlpolicy.OutboundURLError(err),
	})
}
