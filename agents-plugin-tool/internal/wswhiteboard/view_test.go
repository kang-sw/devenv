package wswhiteboard

import "testing"

func topicNames(st *State) []string {
	var names []string
	for _, topic := range BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}).Topics {
		names = append(names, topic.Name)
	}
	return names
}

func wantTopicOrder(t *testing.T, st *State, want ...string) {
	t.Helper()
	got := topicNames(st)
	if len(got) != len(want) {
		t.Fatalf("topic order %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("topic order %v, want %v", got, want)
		}
	}
}

// Within one rank the topic with the most recent activity comes first, and
// the rank stays the primary key.
func TestTopicsOrderByRecencyWithinRank(t *testing.T) {
	st := &State{}
	mustApply(t, st, `{"add":[
		{"class":"policy","topic":"older","title":"a1","options":["x","y"]},
		{"class":"policy","topic":"older","title":"a2","options":["x","y"]}]}`)
	mustApply(t, st, `{"add":[{"class":"policy","topic":"newer","title":"b1","options":["x","y"]}]}`)
	wantTopicOrder(t, st, "newer", "older")

	// Answering an item in the older topic is activity; it stays in the same
	// rank (a2 is still an open policy item) and rises above the newer one.
	mustApply(t, st, `{"answer":[{"id":1,"answer":"a","answer_quote":"go with a"}]}`)
	wantTopicOrder(t, st, "older", "newer")

	// A patch counts as activity too.
	mustApply(t, st, `{"patch":[{"id":3,"title":"b1 reworded"}]}`)
	wantTopicOrder(t, st, "newer", "older")

	// Rank outranks recency: a quiet topic stays below open ones however
	// recently it changed.
	mustApply(t, st, `{"add":[{"class":"technical","topic":"quiet","title":"c1"}]}`)
	mustApply(t, st, `{"answer":[{"id":4,"answer":"ok","answer_quote":"fine"}]}`)
	wantTopicOrder(t, st, "newer", "older", "quiet")
}
