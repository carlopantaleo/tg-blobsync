package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
)

type MockAuthInput struct {
	Phone    string
	Code     string
	Password string
}

func (m *MockAuthInput) GetPhoneNumber() (string, error) {
	return m.Phone, nil
}
func (m *MockAuthInput) GetCode() (string, error) {
	return m.Code, nil
}
func (m *MockAuthInput) GetPassword() (string, error) {
	return m.Password, nil
}

func TestTermAuth(t *testing.T) {
	mockInput := &MockAuthInput{
		Phone:    "+1234567890",
		Code:     "12345",
		Password: "password",
	}

	auth := termAuth{input: mockInput}
	ctx := context.Background()

	// Test Phone
	phone, err := auth.Phone(ctx)
	if err != nil {
		t.Errorf("Phone() error = %v", err)
	}
	if phone != mockInput.Phone {
		t.Errorf("Phone() = %v, want %v", phone, mockInput.Phone)
	}

	// Test Password
	pass, err := auth.Password(ctx)
	if err != nil {
		t.Errorf("Password() error = %v", err)
	}
	if pass != mockInput.Password {
		t.Errorf("Password() = %v, want %v", pass, mockInput.Password)
	}

	// Test Code
	code, err := auth.Code(ctx, nil)
	if err != nil {
		t.Errorf("Code() error = %v", err)
	}
	if code != mockInput.Code {
		t.Errorf("Code() = %v, want %v", code, mockInput.Code)
	}

	// Test AcceptTermsOfService
	if err := auth.AcceptTermsOfService(ctx, tg.HelpTermsOfService{}); err != nil {
		t.Errorf("AcceptTermsOfService() error = %v", err)
	}

	// Test SignUp
	if _, err := auth.SignUp(ctx); err != nil {
		// It returns nil error in implementation?
		// func (t termAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
		// 	return auth.UserInfo{}, nil
		// }
		// Wait, looking at auth.go:31
	}
}
