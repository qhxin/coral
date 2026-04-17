package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestFeishuQuickAckParse_full(t *testing.T) {
	cases := []struct {
		raw      string
		wantMode feishuQuickAckMode
		wantText string
	}{
		{"", feishuQuickAckNone, ""},
		{"   ", feishuQuickAckNone, ""},
		{"false", feishuQuickAckNone, ""},
		{"FALSE", feishuQuickAckNone, ""},
		{"0", feishuQuickAckNone, ""},
		{"true", feishuQuickAckReactionThumbsUp, ""},
		{"TRUE", feishuQuickAckReactionThumbsUp, ""},
		{"1", feishuQuickAckReactionThumbsUp, ""},
		{"收到", feishuQuickAckText, "收到"},
		{"  收到，思考中…  ", feishuQuickAckText, "收到，思考中…"},
		{"1. 小结", feishuQuickAckText, "1. 小结"},
		{"yes", feishuQuickAckText, "yes"},
	}
	for _, tc := range cases {
		m, txt := feishuQuickAckParse(tc.raw)
		if m != tc.wantMode || txt != tc.wantText {
			t.Errorf("feishuQuickAckParse(%q) = mode %v text %q; want mode %v text %q",
				tc.raw, m, txt, tc.wantMode, tc.wantText)
		}
	}
}

func TestFeishuQuickAckModeString_all(t *testing.T) {
	if feishuQuickAckModeString(feishuQuickAckNone) != "off" {
		t.Fatal()
	}
	if feishuQuickAckModeString(feishuQuickAckReactionThumbsUp) != "reaction:THUMBSUP" {
		t.Fatal()
	}
	if feishuQuickAckModeString(feishuQuickAckText) != "text" {
		t.Fatal()
	}
	if feishuQuickAckModeString(feishuQuickAckMode(99)) != "unknown" {
		t.Fatal()
	}
}

func TestFeishuParseImageKey(t *testing.T) {
	k, err := feishuParseImageKey(`{"image_key":"img-v1abc"}`)
	if err != nil || k != "img-v1abc" {
		t.Fatal(k, err)
	}
	if _, err := feishuParseImageKey(""); err == nil {
		t.Fatal("expect error")
	}
}

func TestFeishuParseTextContent(t *testing.T) {
	if _, err := feishuParseTextContent(""); err == nil {
		t.Fatal()
	}
	if _, err := feishuParseTextContent("{"); err == nil {
		t.Fatal()
	}
	s, err := feishuParseTextContent(`{"text":" hi "}`)
	if err != nil || strings.TrimSpace(s) != "hi" {
		t.Fatal(err, s)
	}
}

func TestStrPtr(t *testing.T) {
	if strPtr(nil) != "" {
		t.Fatal()
	}
	x := "a"
	if strPtr(&x) != "a" {
		t.Fatal()
	}
}

func TestEnvIsTruthy(t *testing.T) {
	k := "CORAL_TEST_TRUTHY"
	t.Setenv(k, "")
	if envIsTruthy(k) {
		t.Fatal()
	}
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv(k, v)
		if !envIsTruthy(k) {
			t.Fatal(v)
		}
	}
}

func TestRunFeishuWS_missingEnv(t *testing.T) {
	_ = os.Unsetenv("FEISHU_APP_ID")
	_ = os.Unsetenv("FEISHU_APP_SECRET")
	if err := runFeishuWS(nil); err == nil {
		t.Fatal()
	}
}

func newFeishuTestClient(t *testing.T, handler http.HandlerFunc) (*lark.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cli := lark.NewClient("app", "sec",
		lark.WithOpenBaseUrl(srv.URL),
		lark.WithHttpClient(srv.Client()),
		lark.WithEnableTokenCache(false),
	)
	return cli, srv
}

func TestSendTextMessage_trimsAndTruncates(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}
	cli, _ := newFeishuTestClient(t, ok)
	h := &feishuMsgHandler{httpCli: cli}
	if err := h.sendTextMessage(context.Background(), "cid", "  x  "); err != nil {
		t.Fatal(err)
	}
	if err := h.sendTextMessage(context.Background(), "cid", ""); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 5000)
	if err := h.sendTextMessage(context.Background(), "cid", long); err != nil {
		t.Fatal(err)
	}
}

func TestSendPostMessage_andMarkdownReplies(t *testing.T) {
	n := 0
	cli, _ := newFeishuTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if n == 1 {
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"msg":"fail"}`))
	})
	h := &feishuMsgHandler{httpCli: cli}
	content := `{"zh_cn":{"title":"","content":[[{"tag":"text","text":"p"}]]}}`
	if err := h.sendPostMessage(context.Background(), "cid", content); err != nil {
		t.Fatal(err)
	}
	if err := h.sendPostMessage(context.Background(), "cid", content); err == nil {
		t.Fatal("expected error on bad code")
	}
}

func TestSendMarkdownReplies_fallback(t *testing.T) {
	call := 0
	cli, _ := newFeishuTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		call++
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = w.Write([]byte(`{"code":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	h := &feishuMsgHandler{httpCli: cli}
	if err := h.sendMarkdownReplies(context.Background(), "cid", "# t\n\nhello"); err != nil {
		t.Fatal(err)
	}
}

func TestSendMessageReactionThumbsUp(t *testing.T) {
	cli, _ := newFeishuTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	})
	h := &feishuMsgHandler{httpCli: cli}
	if err := h.sendMessageReactionThumbsUp(context.Background(), " mid "); err != nil {
		t.Fatal(err)
	}
	if err := h.sendMessageReactionThumbsUp(context.Background(), ""); err == nil {
		t.Fatal()
	}
}

func TestFeishuMsgHandler_onMessageReceive_nilSafe(t *testing.T) {
	h := &feishuMsgHandler{}
	if err := h.onMessageReceive(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunFeishuWS_injected(t *testing.T) {
	t.Setenv("FEISHU_APP_ID", "id")
	t.Setenv("FEISHU_APP_SECRET", "sec")
	t.Cleanup(func() { feishuWSRun = nil })
	feishuWSRun = func(ctx context.Context, appID, appSecret string, h *dispatcher.EventDispatcher) error {
		if appID != "id" || appSecret != "sec" || h == nil {
			t.Fatal()
		}
		return errors.New("stub-stop")
	}
	if err := runFeishuWS(&AgentCore{}); err == nil || !strings.Contains(err.Error(), "stub-stop") {
		t.Fatal(err)
	}
}

func spFeishu(s string) *string { return &s }

func TestFeishuOnMessageReceive_earlyExits(t *testing.T) {
	h := &feishuMsgHandler{groupAtOnly: true}
	user := "user"
	app := "app"
	cid := "chat-1"
	mtText := "text"
	mtGroup := "group"
	mtFile := "file"
	good := `{"text":"hello"}`
	bad := `{`

	ev := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: &app},
		Message: &larkim.EventMessage{ChatId: &cid, MessageType: &mtText, Content: &good},
	}}
	if err := h.onMessageReceive(context.Background(), ev); err != nil {
		t.Fatal(err)
	}

	emptyCid := ""
	ev2 := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{ChatId: &emptyCid, MessageType: &mtText, Content: &good},
	}}
	if err := h.onMessageReceive(context.Background(), ev2); err != nil {
		t.Fatal(err)
	}

	ev3 := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender: &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{
			ChatId: &cid, ChatType: &mtGroup, MessageType: &mtText,
			Content: &good,
		},
	}}
	if err := h.onMessageReceive(context.Background(), ev3); err != nil {
		t.Fatal(err)
	}

	ev4 := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{ChatId: &cid, MessageType: &mtText, Content: &bad},
	}}
	if err := h.onMessageReceive(context.Background(), ev4); err != nil {
		t.Fatal(err)
	}

	blanks := `{"text":"  "}`
	ev5 := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{ChatId: &cid, MessageType: &mtText, Content: &blanks},
	}}
	if err := h.onMessageReceive(context.Background(), ev5); err != nil {
		t.Fatal(err)
	}

	emptyImg := `{"image_key":""}`
	mtImg := "image"
	ev6 := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{ChatId: &cid, MessageType: &mtImg, Content: &emptyImg},
	}}
	if err := h.onMessageReceive(context.Background(), ev6); err != nil {
		t.Fatal(err)
	}

	ev7 := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{ChatId: &cid, MessageType: &mtFile, Content: &good},
	}}
	if err := h.onMessageReceive(context.Background(), ev7); err != nil {
		t.Fatal(err)
	}
}

func TestFeishuOnMessageReceive_textStartsWorker(t *testing.T) {
	var n atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	t.Cleanup(api.Close)
	cli := lark.NewClient("a", "b",
		lark.WithOpenBaseUrl(api.URL),
		lark.WithHttpClient(api.Client()),
		lark.WithEnableTokenCache(false),
	)
	llm := newTestOpenAIServer(t, completionJSON("feishu答复", ""))
	t.Cleanup(llm.Close)
	agent := NewAgentCore(newStubOpenAIClient(t, llm.URL, "m", 1), "sys", "", nil, 8000, 500)
	h := &feishuMsgHandler{agent: agent, httpCli: cli}
	user := "user"
	cid := "c-work"
	mt := "text"
	co := `{"text":"你好"}`
	mid := "m1"
	ctp := "p2p"
	ev := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender: &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{
			MessageId: spFeishu(mid), ChatId: spFeishu(cid), ChatType: &ctp,
			MessageType: &mt, Content: &co,
		},
	}}
	if err := h.onMessageReceive(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for n.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n.Load() < 1 {
		t.Fatal("expected outbound Feishu API call")
	}
}

func TestFeishuOnMessageReceive_quickAckTextWhenSet(t *testing.T) {
	var n atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	t.Cleanup(api.Close)
	cli := lark.NewClient("a", "b",
		lark.WithOpenBaseUrl(api.URL),
		lark.WithHttpClient(api.Client()),
		lark.WithEnableTokenCache(false),
	)
	llm := newTestOpenAIServer(t, completionJSON("r", ""))
	t.Cleanup(llm.Close)
	agent := NewAgentCore(newStubOpenAIClient(t, llm.URL, "m", 1), "", "", nil, 8000, 500)
	h := &feishuMsgHandler{
		agent:         agent,
		httpCli:       cli,
		quickAckMode:  feishuQuickAckText,
		quickAckText:  "稍等",
	}
	user := "user"
	cid := "c2"
	mt := "text"
	co := `{"text":"m"}`
	ev := &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender: &larkim.EventSender{SenderType: &user},
		Message: &larkim.EventMessage{
			ChatId: spFeishu(cid), MessageType: &mt, Content: &co,
		},
	}}
	if err := h.onMessageReceive(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for n.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n.Load() < 1 {
		t.Fatal("expected quick ack message")
	}
}

func TestFeishuDownloadMessageImage_emptyArgs(t *testing.T) {
	h := &feishuMsgHandler{}
	if _, err := h.feishuDownloadMessageImage(context.Background(), "", "k"); err == nil {
		t.Fatal()
	}
	if _, err := h.feishuDownloadMessageImage(context.Background(), "mid", ""); err == nil {
		t.Fatal()
	}
	if _, err := h.feishuDownloadMessageImage(context.Background(), "  ", "  "); err == nil {
		t.Fatal()
	}
}
