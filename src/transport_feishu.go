package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// feishuWSRun 非 nil 时替代 larkws.Client.Start（单测注入）。
var feishuWSRun func(ctx context.Context, appID, appSecret string, handler *dispatcher.EventDispatcher) error

// feishuQuickAckMode 表示 FEISHU_QUICK_ACK_TEXT 解析后的即时响应策略。
type feishuQuickAckMode int

const (
	feishuQuickAckNone feishuQuickAckMode = iota
	feishuQuickAckReactionThumbsUp
	feishuQuickAckText
)

// feishuQuickAckParse：空/空白或 false/0 为 none；true/1 为对用户消息点赞（THUMBSUP）；其它非空为发文本文案。
func feishuQuickAckParse(raw string) (feishuQuickAckMode, string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return feishuQuickAckNone, ""
	}
	low := strings.ToLower(s)
	if low == "false" || low == "0" {
		return feishuQuickAckNone, ""
	}
	if low == "true" || low == "1" {
		return feishuQuickAckReactionThumbsUp, ""
	}
	return feishuQuickAckText, s
}

func feishuQuickAckModeString(m feishuQuickAckMode) string {
	switch m {
	case feishuQuickAckNone:
		return "off"
	case feishuQuickAckReactionThumbsUp:
		return "reaction:THUMBSUP"
	case feishuQuickAckText:
		return "text"
	default:
		return "unknown"
	}
}

func runFeishuWS(agent *AgentCore) error {
	appID := strings.TrimSpace(os.Getenv("FEISHU_APP_ID"))
	appSecret := strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	if appID == "" || appSecret == "" {
		return fmt.Errorf("缺少环境变量 FEISHU_APP_ID 或 FEISHU_APP_SECRET")
	}

	httpCli := lark.NewClient(appID, appSecret)
	groupAtOnly := envIsTruthy("FEISHU_GROUP_AT_ONLY")
	quickAckMode, quickAckText := feishuQuickAckParse(os.Getenv("FEISHU_QUICK_ACK_TEXT"))

	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			h := &feishuMsgHandler{
				agent:          agent,
				httpCli:        httpCli,
				groupAtOnly:    groupAtOnly,
				quickAckMode:   quickAckMode,
				quickAckText:   quickAckText,
			}
			return h.onMessageReceive(ctx, event)
		})

	log.Printf("feishu ws: connecting (quick_ack=%s group_at_only=%v)", feishuQuickAckModeString(quickAckMode), groupAtOnly)
	if feishuWSRun != nil {
		return feishuWSRun(context.Background(), appID, appSecret, eventHandler)
	}
	cli := larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)
	return cli.Start(context.Background())
}

type feishuMsgHandler struct {
	agent        *AgentCore
	httpCli      *lark.Client
	groupAtOnly  bool
	quickAckMode feishuQuickAckMode
	quickAckText string
}

func (h *feishuMsgHandler) onMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}
	msg := event.Event.Message
	if event.Event.Sender != nil && event.Event.Sender.SenderType != nil {
		if *event.Event.Sender.SenderType != "user" {
			return nil
		}
	}

	chatID := strPtr(msg.ChatId)
	if chatID == "" {
		return nil
	}
	chatType := strPtr(msg.ChatType)
	if chatType == "group" && h.groupAtOnly {
		if len(msg.Mentions) == 0 {
			return nil
		}
	}

	mt := strPtr(msg.MessageType)
	if mt != "text" {
		log.Printf("feishu: skip message_type=%s chat_id=%s", mt, chatID)
		return nil
	}
	userText, err := feishuParseTextContent(strPtr(msg.Content))
	if err != nil || strings.TrimSpace(userText) == "" {
		return nil
	}
	userText = strings.TrimSpace(userText)

	sess := "feishu-chat-" + chatID
	msgID := strPtr(msg.MessageId)

	switch h.quickAckMode {
	case feishuQuickAckReactionThumbsUp:
		if err := h.sendMessageReactionThumbsUp(ctx, msgID); err != nil {
			log.Printf("feishu: quick ack reaction THUMBSUP failed: %v (chat=%s msg=%s)", err, chatID, msgID)
		}
	case feishuQuickAckText:
		if h.quickAckText != "" {
			if err := h.sendTextMessage(ctx, chatID, h.quickAckText); err != nil {
				log.Printf("feishu: quick ack text failed: %v (chat=%s)", err, chatID)
			}
		}
	}

	go func() {
		bg := context.Background()
		reply, err := h.agent.HandleWithSession(sess, userText)
		if err != nil {
			log.Printf("feishu: HandleWithSession error: %v (chat=%s msg=%s user_in=%q)", err, chatID, msgID, userText)
			_ = h.sendTextMessage(bg, chatID, "处理出错："+err.Error())
			return
		}
		if err := h.sendMarkdownReplies(bg, chatID, reply); err != nil {
			log.Printf("feishu: send reply error: %v", err)
		}
	}()
	return nil
}

func feishuParseTextContent(contentJSON string) (string, error) {
	contentJSON = strings.TrimSpace(contentJSON)
	if contentJSON == "" {
		return "", fmt.Errorf("empty content")
	}
	var m struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(contentJSON), &m); err != nil {
		return "", err
	}
	return m.Text, nil
}

func (h *feishuMsgHandler) sendMarkdownReplies(ctx context.Context, chatID, markdown string) error {
	chunks, err := feishuPostMessageChunks(markdown)
	if err != nil {
		log.Printf("feishu: markdown to post: %v, fallback text", err)
		return h.sendTextMessage(ctx, chatID, markdown)
	}
	for _, content := range chunks {
		if err := h.sendPostMessage(ctx, chatID, content); err != nil {
			log.Printf("feishu: post send failed: %v, fallback full text", err)
			return h.sendTextMessage(ctx, chatID, markdown)
		}
	}
	return nil
}

func (h *feishuMsgHandler) sendPostMessage(ctx context.Context, chatID, contentJSON string) error {
	body := larkim.NewCreateMessageReqBodyBuilder().
		ReceiveId(chatID).
		MsgType("post").
		Content(contentJSON).
		Build()
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(body).
		Build()
	resp, err := h.httpCli.Im.Message.Create(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success() {
		code, msg := 0, ""
		if resp != nil {
			code, msg = resp.Code, resp.Msg
		}
		return fmt.Errorf("im create post: code=%d msg=%s", code, msg)
	}
	return nil
}

func (h *feishuMsgHandler) sendMessageReactionThumbsUp(ctx context.Context, messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("empty message_id for reaction")
	}
	emoji := larkim.NewEmojiBuilder().EmojiType("THUMBSUP").Build()
	body := larkim.NewCreateMessageReactionReqBodyBuilder().ReactionType(emoji).Build()
	req := larkim.NewCreateMessageReactionReqBuilder().
		MessageId(messageID).
		Body(body).
		Build()
	resp, err := h.httpCli.Im.MessageReaction.Create(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success() {
		code, msgStr := 0, ""
		if resp != nil {
			code, msgStr = resp.Code, resp.Msg
		}
		return fmt.Errorf("im message reaction create: code=%d msg=%s", code, msgStr)
	}
	return nil
}

func (h *feishuMsgHandler) sendTextMessage(ctx context.Context, chatID, plain string) error {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		plain = " "
	}
	const textMax = 4500
	if len(plain) > textMax {
		plain = plain[:textMax] + "…"
	}
	content, err := json.Marshal(map[string]string{"text": plain})
	if err != nil {
		return err
	}
	body := larkim.NewCreateMessageReqBodyBuilder().
		ReceiveId(chatID).
		MsgType("text").
		Content(string(content)).
		Build()
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("chat_id").
		Body(body).
		Build()
	resp, err := h.httpCli.Im.Message.Create(ctx, req)
	if err != nil {
		return err
	}
	if resp == nil || !resp.Success() {
		code, msg := 0, ""
		if resp != nil {
			code, msg = resp.Code, resp.Msg
		}
		return fmt.Errorf("im create text: code=%d msg=%s", code, msg)
	}
	return nil
}

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func envIsTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
