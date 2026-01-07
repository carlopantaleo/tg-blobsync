package telegram

import (
	"context"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// termAuth implements auth.UserAuthenticator using the provided AuthInput
type termAuth struct {
	input AuthInput
}

func (t termAuth) Phone(_ context.Context) (string, error) {
	return t.input.GetPhoneNumber()
}

func (t termAuth) Password(_ context.Context) (string, error) {
	return t.input.GetPassword()
}

func (t termAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil // Accept implicitly
}

func (t termAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return t.input.GetCode()
}

func (t termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, nil // Not supported for CLI tool, user must exist
}
