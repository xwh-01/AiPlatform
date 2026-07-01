package user

import (
	"aiplatform/common/code"
	myemail "aiplatform/common/email"
	myredis "aiplatform/common/redis"
	"aiplatform/dao/user"
	"aiplatform/model"
	"aiplatform/utils"
	"aiplatform/utils/myjwt"
	"strings"
)

const maxUsernameGenerateRetries = 3

func Login(account, password string) (string, code.Code) {
	var userInformation *model.User
	var ok bool

	account = strings.TrimSpace(account)
	if account == "" {
		return "", code.CodeInvalidParams
	}

	if ok, userInformation = user.IsExistAccount(account); !ok {
		return "", code.CodeUserNotExist
	}

	if userInformation.Password != utils.MD5(password) {
		return "", code.CodeInvalidPassword
	}

	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)
	if err != nil {
		return "", code.CodeServerBusy
	}

	return token, code.CodeSuccess
}

func Register(email, password, captcha string) (string, code.Code) {
	var userInformation *model.User

	email = strings.TrimSpace(email)
	if email == "" {
		return "", code.CodeInvalidParams
	}

	if ok, _ := user.IsExistEmail(email); ok {
		return "", code.CodeUserExist
	}

	if ok, _ := myredis.CheckCaptchaForEmail(email, captcha); !ok {
		return "", code.CodeInvalidCaptcha
	}

	for i := 0; i < maxUsernameGenerateRetries; i++ {
		username := utils.GetRandomNumbers(11)

		var err error
		userInformation, err = user.Register(username, email, password)
		if err == nil {
			break
		}

		if user.IsDuplicateEntryError(err) {
			if ok, _ := user.IsExistEmail(email); ok {
				return "", code.CodeUserExist
			}
			continue
		}

		return "", code.CodeServerBusy
	}

	if userInformation == nil {
		return "", code.CodeServerBusy
	}

	if err := myemail.SendCaptcha(email, userInformation.Username, user.UserNameMsg); err != nil {
		return "", code.CodeServerBusy
	}

	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)
	if err != nil {
		return "", code.CodeServerBusy
	}

	return token, code.CodeSuccess
}

func SendCaptcha(email string) code.Code {
	email = strings.TrimSpace(email)
	if email == "" {
		return code.CodeInvalidParams
	}

	sendCode := utils.GetRandomNumbers(6)

	if err := myredis.SetCaptchaForEmail(email, sendCode); err != nil {
		return code.CodeServerBusy
	}

	if err := myemail.SendCaptcha(email, sendCode, myemail.CodeMsg); err != nil {
		return code.CodeServerBusy
	}

	return code.CodeSuccess
}
