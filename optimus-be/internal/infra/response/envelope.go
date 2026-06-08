package response

import (
	stderrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"

	apperr "optimus-be/internal/infra/errors"
)

type Envelope struct {
	Code       int    `json:"code"`
	Data       any    `json:"data"`
	Message    string `json:"message"`
	MessageKey string `json:"message_key,omitempty"`
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		Code:    int(apperr.CodeOK),
		Data:    data,
		Message: "",
	})
}

func Error(c *gin.Context, err error) {
	if err == nil {
		c.JSON(http.StatusInternalServerError, Envelope{
			Code:    int(apperr.CodeInternal),
			Data:    nil,
			Message: "internal server error",
		})
		return
	}
	var be *apperr.BizError
	if stderrors.As(err, &be) {
		c.JSON(apperr.HTTPStatus(be.Code), Envelope{
			Code:       int(be.Code),
			Data:       nil,
			Message:    be.Message,
			MessageKey: be.MessageKey,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, Envelope{
		Code:    int(apperr.CodeInternal),
		Data:    nil,
		Message: err.Error(),
	})
}
