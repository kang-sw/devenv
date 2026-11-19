package wswhiteboard

import "testing"

func TestPanelHTMLBlockBalance(t *testing.T) {
	cases := []struct {
		name, payload string
		wants, bads   []string
	}{
		{"indented children", "<div class=\"tkoo\">\n\n        <!-- section -->\n        <div>child</div>\n\n        <p>more</p>\n</div>\n\n**after**", []string{"<div>child</div>", "<p>more</p>", "<strong>after</strong>"}, []string{"<pre><code>"}},
		{"nested", "<div>\n<div>\n\n    <b>nested</b>\n</div>\n\n    <b>outer</b>\n</div>\n\n**after**", []string{"<b>nested</b>", "<b>outer</b>", "<strong>after</strong>"}, []string{"<pre><code>"}},
		{"comment", "<div>\n<!-- <div>\n<div> -->\n</div>\n\n**after**", []string{"<strong>after</strong>"}, nil},
		{"raw text", "<div><script>var x = '<div>';\n\n</script><style>.x:before { content: '<div>'; }</style></div>\n\n**after**", []string{"var x = '<div>';", "<strong>after</strong>"}, nil},
		{"void and self closing", "<div><br><img><input><hr><meta><link><area><base><col><embed><source><track><wbr><x/></div>\n\n**after**", []string{"<strong>after</strong>"}, nil},
		{"unclosed", "<div>\n\n    <b>child</b>\n\n**not markdown**", []string{"<b>child</b>", "**not markdown**"}, []string{"<pre><code>", "<strong>not markdown</strong>"}},
		{"already balanced", "<p>x</p>\n\n**after**", []string{"<strong>after</strong>"}, nil},
		{"optional end tag", "<li>a\n\n**after**", []string{"<strong>after</strong>"}, nil},
		{"multiline opening tag", "<div\n  class=\"a\">\n\n    <b>x</b>\n</div>\n\n**after**", []string{"    <b>x</b>", "<strong>after</strong>"}, []string{"<pre><code>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string((&pageData{}).renderDoc(tc.payload, nil))
			wantAll(t, out, tc.wants...)
			wantNone(t, out, tc.bads...)
		})
	}
}
