package hash

import "golang.org/x/crypto/bcrypt"

// Password 使用 bcrypt 生成密码哈希。
func Password(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验明文密码与哈希是否匹配。
func CheckPassword(hashed, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain)) == nil
}
