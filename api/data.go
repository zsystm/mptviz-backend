package api

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/zsystm/mpt/handlers"
)

// Examples returns a list of examples data for the MPT
func Examples(c echo.Context) error {
	num := c.QueryParams().Get("num")
	if num == "" {
		num = "10"
	}
	numInt, err := strconv.Atoi(num)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid number",
		})
	}
	if numInt < 1 {
		numInt = 10 // default to 10 examples
	}

	return c.JSON(http.StatusOK, handlers.Examples(numInt))
}
