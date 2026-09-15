package service

import "testing"

func TestLoginLimiter_LockAfterMaxAttempts(t *testing.T) {
	l := NewLoginLimiter(3, 15)
	key := "alice"

	for i := 0; i < 3; i++ {
		if l.Locked(key) {
			t.Fatalf("not expected locked at attempt %d", i+1)
		}
		if locked := l.Fail(key); locked != (i == 2) {
			t.Fatalf("attempt %d: locked = %v", i+1, locked)
		}
	}

	if !l.Locked(key) {
		t.Fatal("expected account locked after max attempts")
	}
}

func TestLoginLimiter_SuccessResets(t *testing.T) {
	l := NewLoginLimiter(3, 15)
	key := "alice"

	l.Fail(key)
	l.Fail(key)
	l.Success(key)

	if l.Locked(key) {
		t.Fatal("expected unlocked after success")
	}

	// 重置后从头计数。
	l.Fail(key)
	l.Fail(key)
	if locked := l.Fail(key); !locked {
		t.Fatal("expected locked on 3rd failure after reset")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name    string
		pwd     string
		wantErr bool
	}{
		{"too short", "a1b2c3", true},
		{"letters only", "abcdefgh", true},
		{"digits only", "12345678", true},
		{"valid", "secret123", false},
		{"valid mixed case", "Abc12345", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validatePassword(c.pwd)
			if (err != nil) != c.wantErr {
				t.Fatalf("validatePassword(%q) err = %v, wantErr = %v", c.pwd, err, c.wantErr)
			}
		})
	}
}

func TestPasswordPolicy_Validate(t *testing.T) {
	strict := PasswordPolicy{MinLength: 10, RequireUppercase: true, RequireSpecial: true}

	cases := []struct {
		name    string
		pwd     string
		wantErr bool
	}{
		{"below min length", "Abc123!", true},
		{"missing uppercase", "abc123!xyz", true},
		{"missing special", "Abc123xyz", true},
		{"valid", "Abcdef123!", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := strict.Validate(c.pwd); (err != nil) != c.wantErr {
				t.Fatalf("Validate(%q) err = %v, wantErr = %v", c.pwd, err, c.wantErr)
			}
		})
	}

	// 默认策略保持向后兼容：至少 8 位且含字母与数字。
	if err := DefaultPasswordPolicy().Validate("secret123"); err != nil {
		t.Fatalf("default policy rejected valid password: %v", err)
	}
}
