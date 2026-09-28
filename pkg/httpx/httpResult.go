package httpx

import (
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/perfect-panel/server/pkg/xerr"
)

type HTTPResult struct {
	StatusCode int
	Body       interface{}
}

const (
	paramErrorContextKey   = "__ppanel_param_error"
	handlerErrorContextKey = "__ppanel_handler_error"
)

type paramErrorState struct {
	err error
}

type handlerErrorState struct {
	err error
}

func BuildHTTPResult(resp interface{}, err error) HTTPResult {
	if err == nil {
		return HTTPResult{
			StatusCode: http.StatusOK,
			Body:       Success(resp),
		}
	}

	code := xerr.ERROR
	msg := "Internal Server Error"

	// The first code in the chain is the one to report: xerr.Wrapf only adds
	// an outer code when it is more specific than the one it wraps.
	var e *xerr.CodeError
	if errors.As(err, &e) {
		code = e.GetErrCode()
		msg = e.GetErrMsg()
	}

	return HTTPResult{
		StatusCode: http.StatusOK,
		Body:       Error(code, msg),
	}
}

func BuildParamErrorResult(err error) HTTPResult {
	return HTTPResult{
		StatusCode: http.StatusOK,
		Body:       Error(xerr.InvalidParams, err.Error()),
	}
}

// HttpResult HTTP Result
func HttpResult(ctx *app.RequestContext, resp interface{}, err error) {
	if err != nil {
		// Every response is HTTP 200 with the code in the body, so the access
		// log learns about the failure from here.
		ctx.Set(handlerErrorContextKey, handlerErrorState{err: err})
	}
	result := BuildHTTPResult(resp, err)
	ctx.JSON(result.StatusCode, result.Body)
}

// HandlerErrorFromRequestContext returns the error HttpResult reported for
// ctx, if any.
func HandlerErrorFromRequestContext(ctx *app.RequestContext) error {
	value, ok := ctx.Get(handlerErrorContextKey)
	if !ok {
		return nil
	}
	state, ok := value.(handlerErrorState)
	if !ok {
		return nil
	}
	return state.err
}

// ParamErrorResult Param Error Result
func ParamErrorResult(ctx *app.RequestContext, err error) {
	recordedErr := errors.New(err.Error())
	recordParamError(ctx, recordedErr)
	result := BuildParamErrorResult(err)
	ctx.JSON(result.StatusCode, result.Body)
}

// ParamErrorFromRequestContext returns the parameter error recorded for ctx.
func ParamErrorFromRequestContext(ctx *app.RequestContext) error {
	value, ok := ctx.Get(paramErrorContextKey)
	if !ok {
		return nil
	}
	state, ok := value.(paramErrorState)
	if !ok {
		return nil
	}
	return state.err
}

func recordParamError(ctx *app.RequestContext, err error) {
	ctx.Set(paramErrorContextKey, paramErrorState{err: err})
}
