package main

import (
	"github.com/go-kit/kit/log"
)

type handler struct {
	l              log.Logger
	optionProxyURL string
}
