package session

import (
	"aiplatform/common/aihelper"
	"aiplatform/common/code"
	messageDAO "aiplatform/dao/message"
	sessionDAO "aiplatform/dao/session"
	"aiplatform/model"
	"context"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
)

var ctx = context.Background()

func GetUserSessionsByUserName(userName string) ([]model.SessionInfo, error) {
	sessions, err := sessionDAO.GetSessionsByUserName(userName)
	if err != nil {
		return nil, err
	}

	sessionInfos := make([]model.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		sessionInfos = append(sessionInfos, model.SessionInfo{
			SessionID: s.ID,
			Title:     s.Title,
		})
	}

	return sessionInfos, nil
}

func CreateSessionAndSendMessage(userName string, userQuestion string, modelType string) (string, string, code.Code) {
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		Title:    userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		return "", "", code.CodeServerBusy
	}

	manager := aihelper.GetGlobalManager()
	helper, err := manager.GetOrCreateOrSwitchAIHelper(userName, createdSession.ID, modelType, buildModelConfig(userName))
	if err != nil {
		log.Println("CreateSessionAndSendMessage GetOrCreateOrSwitchAIHelper error:", err)
		return "", "", code.AIModelFail
	}
	helper.MarkHistoryLoaded()

	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
		return "", "", code.AIModelFail
	}

	return createdSession.ID, aiResponse.Content, code.CodeSuccess
}

func CreateStreamSessionOnly(userName string, userQuestion string) (string, code.Code) {
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		Title:    userQuestion,
	}
	createdSession, err := sessionDAO.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	return createdSession.ID, code.CodeSuccess
}

func StreamMessageToExistingSession(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		return code.CodeServerBusy
	}

	helper, err := getReadyHelper(userName, sessionID, modelType)
	if err != nil {
		log.Println("StreamMessageToExistingSession getReadyHelper error:", err)
		return code.AIModelFail
	}

	cb := func(msg string) {
		log.Printf("[SSE] Sending chunk: %s (len=%d)\n", msg, len(msg))
		_, err := writer.Write([]byte("data: " + msg + "\n\n"))
		if err != nil {
			log.Println("[SSE] Write error:", err)
			return
		}
		flusher.Flush()
	}

	_, err = helper.StreamResponse(userName, ctx, cb, userQuestion)
	if err != nil {
		log.Println("StreamMessageToExistingSession StreamResponse error:", err)
		return code.AIModelFail
	}

	_, err = writer.Write([]byte("data: [DONE]\n\n"))
	if err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		return code.AIModelFail
	}
	flusher.Flush()

	return code.CodeSuccess
}

func CreateStreamSessionAndSendMessage(userName string, userQuestion string, modelType string, writer http.ResponseWriter) (string, code.Code) {
	sessionID, code_ := CreateStreamSessionOnly(userName, userQuestion)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSession(userName, sessionID, userQuestion, modelType, writer)
	if code_ != code.CodeSuccess {
		return sessionID, code_
	}

	return sessionID, code.CodeSuccess
}

func ChatSend(userName string, sessionID string, userQuestion string, modelType string) (string, code.Code) {
	helper, err := getReadyHelper(userName, sessionID, modelType)
	if err != nil {
		log.Println("ChatSend getReadyHelper error:", err)
		return "", code.AIModelFail
	}

	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("ChatSend GenerateResponse error:", err)
		return "", code.AIModelFail
	}

	return aiResponse.Content, code.CodeSuccess
}

func GetChatHistory(userName string, sessionID string) ([]model.History, code.Code) {
	helper, err := getHistoryReadyHelper(userName, sessionID)
	if err != nil {
		log.Println("GetChatHistory getHistoryReadyHelper error:", err)
		return nil, code.CodeServerBusy
	}

	messages := helper.GetMessages()
	history := make([]model.History, 0, len(messages))
	for _, msg := range messages {
		history = append(history, model.History{
			IsUser:  msg.IsUser,
			Content: msg.Content,
		})
	}

	return history, code.CodeSuccess
}

func ChatStreamSend(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(userName, sessionID, userQuestion, modelType, writer)
}

func getHistoryReadyHelper(userName string, sessionID string) (*aihelper.AIHelper, error) {
	if err := verifySessionOwner(userName, sessionID); err != nil {
		return nil, err
	}

	manager := aihelper.GetGlobalManager()
	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, "1", buildModelConfig(userName))
	if err != nil {
		return nil, err
	}

	if err := loadSessionHistoryOnce(helper, sessionID); err != nil {
		return nil, err
	}

	return helper, nil
}

func getReadyHelper(userName string, sessionID string, modelType string) (*aihelper.AIHelper, error) {
	if err := verifySessionOwner(userName, sessionID); err != nil {
		return nil, err
	}

	manager := aihelper.GetGlobalManager()
	helper, err := manager.GetOrCreateOrSwitchAIHelper(userName, sessionID, modelType, buildModelConfig(userName))
	if err != nil {
		return nil, err
	}

	if err := loadSessionHistoryOnce(helper, sessionID); err != nil {
		return nil, err
	}

	return helper, nil
}

func loadSessionHistoryOnce(helper *aihelper.AIHelper, sessionID string) error {
	if helper.IsHistoryLoaded() {
		return nil
	}

	msgs, err := messageDAO.GetMessagesBySessionID(sessionID)
	if err != nil {
		return err
	}
	for i := range msgs {
		m := &msgs[i]
		helper.AddMessage(m.Content, m.UserName, m.IsUser, false)
	}
	helper.MarkHistoryLoaded()

	return nil
}

func verifySessionOwner(userName string, sessionID string) error {
	s, err := sessionDAO.GetSessionByID(sessionID)
	if err != nil {
		return err
	}
	if s.UserName != userName {
		return codeError("session does not belong to user")
	}
	return nil
}

func buildModelConfig(userName string) map[string]interface{} {
	return map[string]interface{}{
		"apiKey":    "your-api-key",
		"username":  userName,
		"baseURL":   os.Getenv("OLLAMA_BASE_URL"),
		"modelName": os.Getenv("OLLAMA_MODEL_NAME"),
	}
}

type codeError string

func (e codeError) Error() string {
	return string(e)
}
