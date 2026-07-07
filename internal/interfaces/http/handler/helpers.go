package handler

import (
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/labstack/echo/v4"
)

func httpcallError(c echo.Context, err error) error {
	return httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

func httpcallErrorCustom(c echo.Context, code int, errCode string, msg string) error {
	return httpcallErrorWithDetail(c, code, errCode, msg)
}

func httpcallErrorWithDetail(c echo.Context, code int, errCode string, msg string) error {
	return httpresponse.Error(c, code, errCode, msg)
}

func httpcallSuccess(c echo.Context, code int, data interface{}, meta interface{}) error {
	return httpresponse.Success(c, code, data, meta)
}
