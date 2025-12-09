package replace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrRuntime error = errors.New("runtime error")
var ErrInvalidPattern error = errors.New("invalid pattern")

type UntrustedPattern string
type UntrustedText string
type UntrustedReplacement string

type ReplaceInput struct {
	Pattern     UntrustedPattern     `json:"pattern"`
	Text        UntrustedText        `json:"text"`
	Replacement UntrustedReplacement `json:"replacement"`
}

func (i ReplaceInput) ToJson() ([]byte, error) {
	bytes, err := json.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input to JSON: %w", err)
	}
	return bytes, nil
}

type ReplaceResult struct {
	Replaced string
	Error    error
}

type ErrorDto struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ReplaceResultDto struct {
	ReplacedText string    `json:"replaced_text"`
	Error        *ErrorDto `json:"error"`
}

func ReplaceResultDtoFromJson(j []byte) (d ReplaceResultDto, e error) {
	e = json.Unmarshal(j, &d)
	return
}

const (
	ErrCodeInvalidPattern = 2
)

func (d ReplaceResultDto) ToResult() ReplaceResult {
	var err error
	if d.Error != nil {
		switch d.Error.Code {
		case ErrCodeInvalidPattern:
			err = fmt.Errorf("%w: %s", ErrInvalidPattern, d.Error.Message)
		default:
			err = fmt.Errorf("%w: %s", ErrRuntime, d.Error.Message)
		}
	}
	return ReplaceResult{
		Replaced: d.ReplacedText,
		Error:    err,
	}
}

type Replacer func(context.Context, ReplaceInput) ReplaceResult
