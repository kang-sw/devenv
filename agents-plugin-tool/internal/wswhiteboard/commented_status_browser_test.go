package wswhiteboard

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestCommentedStatusCaptureFixture(t *testing.T) {
	dir := os.Getenv("WS_COMMENTED_CAPTURE_DIR")
	if dir == "" {
		t.Skip("set WS_COMMENTED_CAPTURE_DIR")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	st := &State{}
	apply := func(xml string, minute int) {
		t.Helper()
		_, p := Apply(st, xml, Env{Now: time.Date(2026, 11, 16, 9, minute, 0, 0, time.UTC)})
		if len(p) > 0 {
			t.Fatalf("%s: %v", xml, p)
		}
	}
	apply(`<wb:topic name="permissions" label="권한 프로필"><wb:log>생성 폼과 경고 대화상자 결정 필요</wb:log></wb:topic>`, 0)
	apply(`<wb:topic name="permissions"><wb:add class="policy"><wb:title>생성 폼의 기본 권한 수준은?</wb:title><wb:option>기본(기존 동작)</wb:option><wb:option>작업 폴더 쓰기</wb:option><wb:recommend>a</wb:recommend></wb:add></wb:topic>`, 0)
	apply(`<wb:topic name="permissions"><wb:add class="policy"><wb:title>전체 권한 경고 문구의 길이는?</wb:title><wb:why>위험을 빠짐없이 알립니다.</wb:why><wb:option>한 줄 요약</wb:option><wb:option>네 줄 상세</wb:option><wb:recommend>b</wb:recommend></wb:add></wb:topic>`, 0)
	apply(`<wb:topic name="permissions"><wb:add class="policy"><wb:title>권한 칩을 세션 머리에 둘까요?</wb:title><wb:option>둔다</wb:option><wb:option>두지 않는다</wb:option></wb:add></wb:topic>`, 0)
	apply(`<wb:answer id="3" value="a">둔다</wb:answer>`, 20)
	apply(`<wb:answer id="2" value="">둘 다 길어요. 한 줄로 줄이고 자세한 건 "자세히" 링크로 빼 주세요.</wb:answer>`, 24)
	assets, err := PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	save := func(name string) {
		t.Helper()
		v := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "capture", Location: time.UTC})
		v.Lang = "ko"
		regions, err := RenderRegions(v)
		if err != nil {
			t.Fatal(err)
		}
		page := `<!doctype html><html lang="ko"><head><meta charset="utf-8"><style>` + string(assets.CSS) + `</style></head><body>` + string(assets.Sprite) + `<main>` + string(regions.Board) + `</main><script>` + string(wsi18n.Script(v.Lang)) + `</script><script>` + string(assets.AnswersScript) + `</script><script>(function(){` + string(assets.FormScript) + `})();</script></body></html>`
		if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(page), 0644); err != nil {
			t.Fatal(err)
		}
	}
	save("fresh")
	v := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "capture", Location: time.UTC})
	v.Lang = "ko"
	a, err := loadAssets()
	if err != nil {
		t.Fatal(err)
	}
	d := newPageData(v, a)
	var settled bytes.Buffer
	if err := a.templateFor(v.Lang).ExecuteTemplate(&settled, "settled", d.TopicOf(v.Topics[0])); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settled.html"), []byte(`<!doctype html><html lang="ko"><meta charset="utf-8"><style>`+string(assets.CSS)+`</style><body>`+string(assets.Sprite)+`<main class="topic">`+settled.String()+`</main></body></html>`), 0644); err != nil {
		t.Fatal(err)
	}

	apply(`<wb:topic name="permissions"><wb:log>경고 문구 의견 받음, 시안 다시 준비</wb:log></wb:topic>`, 26)
	save("history")
	apply(`<wb:patch id="2" status="open"/>`, 41)
	apply(`<wb:topic name="permissions"><wb:log>경고 문구 시안 다시 질문</wb:log></wb:topic>`, 42)
	save("reask")
}
