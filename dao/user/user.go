package user

import (
	"aiplatform/common/mysql"
	"aiplatform/model"
	"aiplatform/utils"
	"context"

	"gorm.io/gorm"
)

const (
	CodeMsg     = "aiplatform验证码如下(验证码仅限于2分钟有效): "
	UserNameMsg = "aiplatform的账号如下，请保留好，后续可以用账号进行登录 "
)

var ctx = context.Background()

// 这边只能通过账号进行登录
func IsExistUser(username string) (bool, *model.User) {

	user, err := mysql.GetUserByUsername(username)

	if err == gorm.ErrRecordNotFound || user == nil {
		return false, nil
	}

	return true, user
}

func IsExistEmail(email string) (bool, *model.User) {
	user, err := mysql.GetUserByEmail(email)

	if err == gorm.ErrRecordNotFound || user == nil {
		return false, nil
	}

	return true, user
}

func IsExistAccount(account string) (bool, *model.User) {
	user, err := mysql.GetUserByAccount(account)

	if err == gorm.ErrRecordNotFound || user == nil {
		return false, nil
	}

	return true, user
}

func Register(username, email, password string) (*model.User, error) {
	if user, err := mysql.InsertUser(&model.User{
		Email:    email,
		Name:     username,
		Username: username,
		Password: utils.MD5(password),
	}); err != nil {
		return nil, err
	} else {
		return user, nil
	}
}

func IsDuplicateEntryError(err error) bool {
	return mysql.IsDuplicateEntryError(err)
}
