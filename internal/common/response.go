package common

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func BadRequest(message string) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusBadRequest, ErrorResponse{Error: message})
}

func NotFound(message string) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusNotFound, ErrorResponse{Error: message})
}

func Internal(message string) *echo.HTTPError {
	return echo.NewHTTPError(http.StatusInternalServerError, ErrorResponse{Error: message})
}
