package wswhiteboard

import (
	"reflect"
	"testing"
)

func TestFileImagePathsIncludesDeferredAndHistoricalContent(t *testing.T) {
	s := &State{
		Topics: []TopicDoc{{Name: "closed", Body: "![body](shots/body.png)\n\n<IMG src=\"shots/raw.png\">", Entries: []Entry{
			{ID: "e1", Kind: EntryPanel, Text: "![old](shots/old.png) ![remote](https://example.com/a.png)"},
			{ID: "e2", Kind: EntryPanel, Text: "current without images"},
			{ID: "e3", Kind: EntryAgent, Text: "`![code](shots/code.png)`\n\n```html\n<img src=\"shots/code2.png\">\n```"},
		}}},
		Items:        []Item{{Rationale: "![missing](shots/missing.png)", Options: []string{"![option](shots/option.png)", "![dup](shots/body.png)"}}},
		ClosedTopics: []string{"closed"},
	}
	want := []string{"shots/body.png", "shots/missing.png", "shots/old.png", "shots/option.png", "shots/raw.png"}
	if got := FileImagePaths(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("file image dependencies = %v, want %v", got, want)
	}
}

func TestHistoryImageStampRefreshesRetainedFramesUnderNode(t *testing.T) {
	runLazyHistoryScript(t, `(async function(){
  function imageTopic(version,inGap){var frame='<div class="doc-frame" data-doc-key="entry:e1" data-relay="/s/r/image?path=a.png&amp;v='+version+'" data-srcdoc="image-version-'+version+'"></div><img src="/s/r/image?path=b.png&amp;v='+version+'">';return '<section class="topic closed" data-topic="t">'+(inGap?'<template data-history-gap>'+frame+'</template>':frame)+'</section>';}
  root=templates(parse(imageTopic('one')+'<template data-history-images="one"></template>'));
  var frame=root.querySelector('.doc-frame'), inline=root.querySelector('img');frame.style.height='240px';
  await window.wbHistory.refreshImages(document);
  check('initial image stamp does not fetch',requests.length,0);
  // The topic node is retained by the board morph; only the shell stamp changes.
  var section=root.querySelector('section.topic');
  root.querySelector('template[data-history-images]').setAttribute('data-history-images','two');
  var pending=window.wbHistory.refreshImages(document);await drain();
  check('stamp change fetches already hydrated topic',requests.map(r=>r.query),['name=t']);
  requests[0].resolve([{name:'t',html:imageTopic('two')}]);await pending;
  var refreshed=root.querySelector('.doc-frame');
  check('topic node remains retained',root.querySelector('section.topic')===section,true);
  check('image frame gets new URL',refreshed.getAttribute('data-relay'),'/s/r/image?path=a.png&v=two');
  check('image frame gets new document',refreshed.getAttribute('data-srcdoc'),'image-version-two');
  check('image frame keeps height',refreshed.style.height,'240px');
  check('old frame replaced',refreshed===frame,false);
  check('inline history image keeps identity',root.querySelector('img')===inline,true);
  check('inline history image gets new URL',inline.getAttribute('src'),'/s/r/image?path=b.png&v=two');
  await window.wbHistory.refreshImages(document);
  check('stable stamp does not refetch',requests.length,1);
  // A name= prefetch may have begun before the image edit. Its old
  // response must not satisfy the later image refresh.
  section.appendChild(new E('div',{'class':'entry-body','data-lazy':''}));
  var older=window.wbHistory.hydrateAll(document);await drain();
  root.querySelector('template[data-history-images]').setAttribute('data-history-images','three');
  var latest=window.wbHistory.refreshImages(document);await drain();
  check('image refresh waits for existing prefetch',requests.length,2);
  requests[1].resolve([{name:'t',html:imageTopic('two')}]);await drain();
  check('image refresh fetches after stale prefetch',requests.length,3);
  requests[2].resolve([{name:'t',html:imageTopic('three',true)}]);await Promise.all([older,latest]);
  check('late edit gets current URL',root.querySelector('.doc-frame').getAttribute('data-relay'),'/s/r/image?path=a.png&v=three');
  // Different query keys cannot hydrate stale image URLs either.
  window.wbHistory.loadTarget(root,'e99');var target=window.wbHistory.loading('e99');await drain();
  root.querySelector('template[data-history-images]').setAttribute('data-history-images','four');
  var fourth=window.wbHistory.refreshImages(document);await drain();
  check('target request predates image refresh',requests.slice(3).map(r=>r.query),['target=e99','name=t']);
  requests[3].resolve([{name:'t',html:imageTopic('three')}]);await drain();
  check('stale target response refetches independently',requests[5].query,'target=e99');
  requests[4].resolve([{name:'t',html:imageTopic('four')}]);
  requests[5].resolve([{name:'t',html:imageTopic('four')}]);await Promise.all([fourth,target]);
  // Failed refreshes keep the stamp pending and schedule a bounded retry.
  var fetcher=window.wbHistoryFetch, retries=[];
  window.setTimeout=function(fn){retries.push(fn);return retries.length;};
  window.wbHistoryFetch=function(){return Promise.reject(Error('temporary read failure'));};
  root.querySelector('template[data-history-images]').setAttribute('data-history-images','five');
  await window.wbHistory.refreshImages(document);
  check('failed image refresh schedules retry',retries.length,1);
  window.wbHistoryFetch=fetcher;retries[0]();await drain();
  requests[6].resolve([{name:'t',html:imageTopic('five')}]);await drain();
  check('retry updates retained frame without another board event',root.querySelector('.doc-frame').getAttribute('data-relay'),'/s/r/image?path=a.png&v=five');
  scriptDone=true;
  if(bad.length)throw Error(bad.join('\n'));
})().catch(e=>{console.error(e);process.exitCode=1;});`)
}
