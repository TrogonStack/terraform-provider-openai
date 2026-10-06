package provider

import (
	"errors"
	"net/http"
)

func isNotFound(err error) bool {
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.statusCode == http.StatusNotFound
}
