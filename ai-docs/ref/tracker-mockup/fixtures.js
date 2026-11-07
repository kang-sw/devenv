// Fixtures for the tracker mockup v3: per store, a roles file (genesis, including
// the priority levels), one commit list per clone ref (stream "<member>.<clone>",
// ref refs/heads/tracker/m/<member>/<clone>), and `late`: commits their writer has
// pushed that no other clone has fetched yet (they arrive on the next sync).
// Commit = { stream, ts, signer, seen: { stream: lastSeq }, events: [{ seq, ts?, type,
// origin?, payload }] }; an event without its own ts uses the commit's ts.
// `clones` lists extra clones, `now` is the page clock's start, and a store with no
// remote keeps its commits local (unpushed) on its single clone.
// Loaded as a plain script so index.html opens from file:// (fetch of local JSON is
// blocked there). Edit freely, or edit the Data card in the page; reload to reset.
window.TRACKER_FIXTURES = {
  stores: [
    {
      name: "team-alpha",
      label: "team-alpha (team store)",
      defaultViewer: "maya.pc",
      now: "2026-11-06T09:00:00Z",
      clones: ["maya.pc", "maya.lap"],
      sync: {"remote":"git@git.example.com:team-alpha/tracker.git","syncedAt":"2026-11-05T12:00:00Z"},
      roles: {
        "admins": ["ada","bo"],
        "priority": {"names": ["알림", "업무", "긴급"], "default": 1},
        "members": [
          {"id":"ada","name":"Ada Park","keys":["ada-k1"],"leads":[]},
          {"id":"maya","name":"Maya Ortiz","keys":["maya-k1"],"leads":["ada"]},
          {"id":"raj","name":"Raj Rao","keys":["raj-k1"],"leads":["ada"]},
          {"id":"bo","name":"Bo Lindqvist","keys":["bo-k1"],"leads":["ada"]},
          {"id":"lena","name":"Lena Weiss","keys":["lena-k1"],"leads":["maya"]},
          {"id":"omar","name":"Omar Haddad","keys":["omar-k1"],"leads":["maya"]},
          {"id":"ivy","name":"Ivy Chen","keys":["ivy-k1"],"leads":["maya"]},
          {"id":"jon","name":"Jon Ek","keys":["jon-k1"],"leads":["maya"]},
          {"id":"kai","name":"Kai Mori","keys":["kai-k1"],"leads":["maya"]},
          {"id":"sven","name":"Sven Berg","keys":["sven-k1"],"leads":["raj"]},
          {"id":"tia","name":"Tia Moreau","keys":["tia-k1"],"leads":["raj"]},
          {"id":"noa","name":"Noa Fisher","keys":["noa-k1"],"leads":["raj"]},
          {"id":"uma","name":"Uma Iyer","keys":["uma-k1"],"leads":["bo"]}
        ]
      },
      commits: {
        "ada.pc": [
          {"stream":"ada.pc","ts":"2026-11-02T09:00:00Z","signer":"ada-k1","seen":{},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Ship search v2","body":"Replace the v1 indexer and the query path.","project":"search-service","due":"2026-11-20","priority":2}},{"seq":2,"type":"item.assign","payload":{"item":"ada.pc#1","mode":"all","levels":1,"completion":"assigner"}}]},
          {"stream":"ada.pc","ts":"2026-11-02T09:20:00Z","signer":"ada-k1","seen":{},"events":[{"seq":3,"type":"item.create","payload":{"kind":"topic","title":"Q4 priorities","body":"Search v2 first, CI hygiene second, training by end of month."}}]},
          {"stream":"ada.pc","ts":"2026-11-02T10:30:00Z","signer":"ada-k1","seen":{"maya.pc":3,"raj.pc":3},"events":[{"seq":4,"type":"comment.add","payload":{"item":"ada.pc#1","body":"Yes - maya, please publish the format spec by Wednesday.","reply_to":"raj.pc#3"}}]},
          {"stream":"ada.pc","ts":"2026-11-02T10:40:00Z","signer":"ada-k1","seen":{"maya.pc":3,"raj.pc":3},"events":[{"seq":5,"type":"item.edit","payload":{"item":"ada.pc#1","title":"Ship search v2 (index + query path)","body":"Replace the v1 indexer and the query path. Index format frozen on Wednesday."}}]},
          {"stream":"ada.pc","ts":"2026-11-02T17:00:00Z","signer":"ada-k1","seen":{"bo.pc":2,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":6,"type":"item.create","payload":{"kind":"task","title":"Security training","body":"Finish the module before the 30th.","due":"2026-11-30","priority":0}},{"seq":7,"type":"item.assign","payload":{"item":"ada.pc#6","mode":"all","levels":2,"completion":"assignee"}}]},
          {"stream":"ada.pc","ts":"2026-11-02T17:30:00Z","signer":"ada-k1","seen":{"bo.pc":2,"ivy.pc":2,"jon.pc":1,"kai.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":8,"type":"item.create","payload":{"kind":"todo","title":"Book offsite venue","due":"2026-11-14","priority":0}},{"seq":9,"type":"item.assign","payload":{"item":"ada.pc#8","mode":"chosen","completion":"assignee","people":["ada"]}}]},
          {"stream":"ada.pc","ts":"2026-11-03T08:30:00Z","signer":"ada-k1","seen":{"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":10,"type":"item.accept","payload":{"item":"kai.pc#2"}}]},
          {"stream":"ada.pc","ts":"2026-11-03T09:00:00Z","signer":"ada-k1","seen":{"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":11,"type":"roles.add_member","payload":{"member":{"id":"pia","name":"Pia Novak","keys":["pia-k1"],"leads":["raj"]}}}]},
          {"stream":"ada.pc","ts":"2026-11-04T15:00:00Z","signer":"ada-k1","seen":{"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":18,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":12,"type":"item.close","payload":{"item":"kai.pc#2","note":"budget approved for Q4"}}]},
          {"stream":"ada.pc","ts":"2026-11-05T09:00:00Z","signer":"ada-k1","seen":{"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":19,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":13,"type":"comment.add","payload":{"item":"ada.pc#3","body":"CI hygiene moves to December.","reply_to":"maya.pc#3"}}]},
          {"stream":"ada.pc","ts":"2026-11-05T09:30:00Z","signer":"ada-k1","seen":{"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":19,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":14,"type":"item.close","payload":{"item":"ada.pc#8"}}]}
        ],
        "maya.pc": [
          {"stream":"maya.pc","ts":"2026-11-02T09:40:00Z","signer":"maya-k1","seen":{"ada.pc":3},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Index builder","parent":"ada.pc#1","project":"search-service","due":"2026-11-13","priority":2}},{"seq":2,"type":"item.assign","payload":{"item":"maya.pc#1","mode":"chosen","completion":"assignee","people":["lena","omar"]}}]},
          {"stream":"maya.pc","ts":"2026-11-02T09:45:00Z","signer":"maya-k1","seen":{"ada.pc":3},"events":[{"seq":3,"type":"comment.add","payload":{"item":"ada.pc#3","body":"Can CI hygiene slip to December? Search v2 needs my whole part."}}]},
          {"stream":"maya.pc","ts":"2026-11-02T11:00:00Z","signer":"maya-k1","seen":{"ada.pc":5,"raj.pc":5},"events":[{"seq":4,"type":"comment.hide","payload":{"comment":"raj.pc#4"}}]},
          {"stream":"maya.pc","ts":"2026-11-02T11:10:00Z","signer":"maya-k1","seen":{"ada.pc":5,"raj.pc":5},"events":[{"seq":5,"type":"item.create","payload":{"kind":"task","title":"Tokenizer","parent":"maya.pc#1","due":"2026-11-06","priority":1}},{"seq":6,"type":"item.assign","payload":{"item":"maya.pc#5","mode":"chosen","completion":"assigner","people":["ivy","jon"]}}]},
          {"stream":"maya.pc","ts":"2026-11-02T11:15:00Z","signer":"maya-k1","seen":{"ada.pc":5,"raj.pc":5},"events":[{"seq":7,"type":"item.create","payload":{"kind":"task","title":"Shard writer docs","parent":"maya.pc#1","due":"2026-11-10","priority":1}},{"seq":8,"type":"item.assign","payload":{"item":"maya.pc#7","mode":"chosen","completion":"assigner","people":["omar"]}}]},
          {"stream":"maya.pc","ts":"2026-11-02T13:30:00Z","signer":"maya-k1","seen":{"ada.pc":5,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"omar.pc":2,"raj.pc":5},"events":[{"seq":9,"type":"item.confirm","payload":{"item":"maya.pc#5","assignee":"ivy"}}]},
          {"stream":"maya.pc","ts":"2026-11-02T20:40:00Z","signer":"maya-k1","seen":{"ada.pc":9,"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":10,"type":"item.edit","payload":{"item":"ada.pc#1","title":"Ship search v2 by Friday"}}]},
          {"stream":"maya.pc","ts":"2026-11-03T10:20:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"noa.pc":2,"omar.pc":2,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":11,"type":"item.return","payload":{"item":"sven.pc#3","assignee":"jon","note":"jon is on the tokenizer rework this week; please ask lena."}}]},
          {"stream":"maya.pc","ts":"2026-11-03T11:30:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"noa.pc":2,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":3},"events":[{"seq":12,"type":"item.reject","payload":{"item":"maya.pc#5","assignee":"jon","note":"stemmer tests missing for Korean input"}}]},
          {"stream":"maya.pc","ts":"2026-11-03T15:00:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":4,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":13,"type":"item.reject","payload":{"item":"maya.pc#7","assignee":"omar","note":"needs the migration section"}}]},
          {"stream":"maya.pc","ts":"2026-11-03T15:10:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":4,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":14,"type":"comment.add","origin":"agent","payload":{"item":"maya.pc#1","body":"Status digest: tokenizer back to jon for Korean tests; shard writer docs need a migration section; omar has been dark since noon."}}]},
          {"stream":"maya.pc","ts":"2026-11-04T10:00:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":4,"jon.pc":5,"kai.pc":3,"lena.pc":4,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":15,"type":"item.unassign","payload":{"item":"maya.pc#1","assignee":"omar"}},{"seq":16,"type":"comment.add","payload":{"item":"maya.pc#1","body":"Unassigned omar from the index builder while he is dark; lena covers the writer."}}]},
          {"stream":"maya.pc","ts":"2026-11-04T13:00:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":7,"ivy.pc":4,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":17,"type":"item.confirm","payload":{"item":"maya.pc#5","assignee":"jon"}}]},
          {"stream":"maya.pc","ts":"2026-11-04T13:30:00Z","signer":"maya-k1","seen":{"ada.pc":11,"bo.pc":7,"ivy.pc":4,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":18,"type":"item.reopen","payload":{"item":"maya.pc#5","assignee":"ivy","note":"regression in emoji tokenization"}}]},
          {"stream":"maya.pc","ts":"2026-11-04T16:00:00Z","signer":"maya-k1","seen":{"ada.pc":12,"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":19,"type":"comment.add","origin":"agent","payload":{"item":"ada.pc#1","body":"Weekly digest: tokenizer reopened for emoji; query path blocked below on the cache layer (pia waits for the redis approval)."}}]},
          {"stream":"maya.pc","ts":"2026-11-04T21:30:00Z","signer":"maya-k1","seen":{"ada.pc":12,"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":20,"type":"item.edit","payload":{"item":"maya.pc#1","body":"Build the v2 index: layout, shard writer, tokenizer, compaction."}}]}
        ],
        "raj.pc": [
          {"stream":"raj.pc","ts":"2026-11-02T10:00:00Z","signer":"raj-k1","seen":{"ada.pc":3,"maya.pc":3},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Query path rewrite","parent":"ada.pc#1","project":"search-service","due":"2026-11-18","priority":2}},{"seq":2,"type":"item.assign","payload":{"item":"raj.pc#1","mode":"chosen","completion":"assignee","people":["sven","tia"]}}]},
          {"stream":"raj.pc","ts":"2026-11-02T10:10:00Z","signer":"raj-k1","seen":{"ada.pc":3,"maya.pc":3},"events":[{"seq":3,"type":"comment.add","payload":{"item":"ada.pc#1","body":"The query path depends on the new index format; can we freeze it by Wednesday?"}}]},
          {"stream":"raj.pc","ts":"2026-11-02T10:50:00Z","signer":"raj-k1","seen":{"ada.pc":5,"maya.pc":3},"events":[{"seq":4,"type":"comment.add","payload":{"item":"ada.pc#3","body":"Moving the cache layer to next week."}}]},
          {"stream":"raj.pc","ts":"2026-11-02T10:52:00Z","signer":"raj-k1","seen":{"ada.pc":5,"maya.pc":3},"events":[{"seq":5,"type":"comment.hide","payload":{"comment":"raj.pc#4"}}]},
          {"stream":"raj.pc","ts":"2026-11-02T18:30:00Z","signer":"raj-k1","seen":{"ada.pc":9,"bo.pc":2,"ivy.pc":3,"jon.pc":1,"kai.pc":1,"lena.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":6,"type":"item.create","origin":"agent-auto","payload":{"kind":"task","title":"Draft perf notes"}}]},
          {"stream":"raj.pc","ts":"2026-11-03T09:30:00Z","signer":"raj-k1","seen":{"ada.pc":11,"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":7,"type":"item.create","payload":{"kind":"task","title":"Cache layer","parent":"raj.pc#1","due":"2026-11-12","priority":2}},{"seq":8,"type":"item.assign","payload":{"item":"raj.pc#7","mode":"chosen","completion":"assignee","people":["pia"]}}]},
          {"stream":"raj.pc","ts":"2026-11-05T13:00:00Z","signer":"raj-k1","seen":{"ada.pc":14,"bo.pc":8,"ivy.pc":5,"jon.pc":8,"kai.pc":4,"lena.pc":5,"mallory.pc":1,"maya.lap":3,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"sven.pc":4,"tia.pc":5,"uma.pc":5},"events":[{"seq":9,"type":"item.accept","payload":{"item":"lena.pc#2","note":"I can take it this week"}}]},
          {"stream":"raj.pc","ts":"2026-11-05T13:30:00Z","signer":"raj-k1","seen":{"ada.pc":14,"bo.pc":8,"ivy.pc":5,"jon.pc":8,"kai.pc":4,"lena.pc":5,"mallory.pc":1,"maya.lap":3,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"sven.pc":4,"tia.pc":5,"uma.pc":5},"events":[{"seq":10,"type":"item.handoff","payload":{"item":"lena.pc#2","to":"sven","note":"sven owns the bench rig now"}}]}
        ],
        "bo.pc": [
          {"stream":"bo.pc","ts":"2026-11-02T15:00:00Z","signer":"bo-k1","seen":{"ada.pc":5,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":1,"tia.pc":1},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Rotate CI keys","project":"infra","due":"2026-11-03","priority":2}},{"seq":2,"type":"item.assign","payload":{"item":"bo.pc#1","mode":"chosen","completion":"assignee","people":["uma"]}}]},
          {"stream":"bo.pc","ts":"2026-11-02T20:00:00Z","signer":"bo-k1","seen":{"ada.pc":9,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":1,"mallory.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":3,"type":"roles.set_lead","payload":{"member":"ada","leads":["uma"]}}]},
          {"stream":"bo.pc","ts":"2026-11-02T20:10:00Z","signer":"bo-k1","seen":{"ada.pc":9,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":1,"mallory.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":4,"type":"roles.set_lead","payload":{"member":"sven","leads":["maya","raj"]}}]},
          {"stream":"bo.pc","ts":"2026-11-03T10:05:00Z","signer":"bo-k1","seen":{"ada.pc":11,"ivy.pc":3,"jon.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"pia.pc":1,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":5,"type":"item.return","payload":{"item":"sven.pc#3","assignee":"jon","note":"jon is needed on CI"}}]},
          {"stream":"bo.pc","ts":"2026-11-04T11:00:00Z","signer":"bo-k1","seen":{"ada.pc":11,"ivy.pc":4,"jon.pc":5,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":16,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":6,"type":"roles.revoke_key","payload":{"member":"jon","key":"jon-k1"}},{"seq":7,"type":"roles.add_key","payload":{"member":"jon","key":"jon-k2"}}]},
          {"stream":"bo.pc","ts":"2026-11-05T11:00:00Z","signer":"bo-k1","seen":{"ada.pc":14,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":19,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":5,"uma.pc":5},"events":[{"seq":8,"type":"comment.add","payload":{"item":"raj.pc#7","body":"Approved in principle; the infra ticket is open.","reply_to":"tia.pc#5"}}]},
          {"stream":"bo.pc","ts":"2026-11-05T11:30:00Z","signer":"bo-k1","seen":{"ada.pc":14,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":19,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":5,"uma.pc":5},"events":[{"seq":12,"type":"item.create","payload":{"kind":"todo","title":"Seq gap demo"}}]}
        ],
        "lena.pc": [
          {"stream":"lena.pc","ts":"2026-11-02T11:20:00Z","signer":"lena-k1","seen":{"ada.pc":5,"maya.pc":8,"raj.pc":5},"events":[{"seq":1,"type":"item.status","payload":{"item":"maya.pc#1","status":"doing","note":"index layout drafted"}}]},
          {"stream":"lena.pc","ts":"2026-11-02T20:20:00Z","signer":"lena-k1","seen":{"ada.pc":9,"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"mallory.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":2,"type":"item.create","payload":{"kind":"task","title":"Benchmark harness","due":"2026-11-21","priority":1}},{"seq":3,"type":"item.assign","payload":{"item":"lena.pc#2","mode":"chosen","completion":"assignee","people":["raj"]}}]},
          {"stream":"lena.pc","ts":"2026-11-03T12:10:00Z","signer":"lena-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"mallory.pc":1,"maya.pc":12,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":4,"type":"item.accept","origin":"agent-auto","payload":{"item":"noa.pc#3"}}]},
          {"stream":"lena.pc","ts":"2026-11-04T10:30:00Z","signer":"lena-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":4,"jon.pc":5,"kai.pc":3,"mallory.pc":1,"maya.pc":16,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":5,"type":"item.status","payload":{"item":"maya.pc#1","status":"doing","note":"picked up the shard writer"}}]}
        ],
        "omar.pc": [
          {"stream":"omar.pc","ts":"2026-11-02T12:00:00Z","signer":"omar-k1","seen":{"ada.pc":5,"ivy.pc":1,"jon.pc":1,"lena.pc":1,"maya.pc":8,"raj.pc":5},"events":[{"seq":1,"type":"item.status","origin":"agent-auto","payload":{"item":"maya.pc#1","status":"doing","note":"shard writer done, docs next"}},{"seq":2,"type":"item.status","payload":{"item":"maya.pc#7","status":"doing"}}]},
          {"stream":"omar.pc","ts":"2026-11-03T10:30:00Z","signer":"omar-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":11,"noa.pc":2,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":3,"type":"item.close","payload":{"item":"maya.pc#7","note":"docs drafted"}}]}
        ],
        "ivy.pc": [
          {"stream":"ivy.pc","ts":"2026-11-02T11:30:00Z","signer":"ivy-k1","seen":{"ada.pc":5,"lena.pc":1,"maya.pc":8,"raj.pc":5},"events":[{"seq":1,"type":"item.status","origin":"agent-auto","payload":{"item":"maya.pc#5","status":"doing","note":"regex tokenizer drafted"}}]},
          {"stream":"ivy.pc","ts":"2026-11-02T13:00:00Z","signer":"ivy-k1","seen":{"ada.pc":5,"jon.pc":1,"lena.pc":1,"maya.pc":8,"omar.pc":2,"raj.pc":5},"events":[{"seq":2,"type":"item.close","origin":"agent-auto","payload":{"item":"maya.pc#5","note":"tests green"}}]},
          {"stream":"ivy.pc","ts":"2026-11-02T17:40:00Z","signer":"ivy-k1","seen":{"ada.pc":9,"bo.pc":2,"jon.pc":1,"kai.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":3,"type":"item.status","origin":"agent-auto","payload":{"item":"ada.pc#6","status":"doing","note":"module 2 of 5"}}]},
          {"stream":"ivy.pc","ts":"2026-11-03T17:00:00Z","signer":"ivy-k1","seen":{"ada.pc":11,"bo.pc":5,"jon.pc":5,"kai.pc":3,"lena.pc":4,"mallory.pc":1,"maya.pc":14,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":4,"type":"comment.add","payload":{"item":"maya.pc#5","body":"I can pair with jon on the Korean fixtures."}}]},
          {"stream":"ivy.pc","ts":"2026-11-04T14:00:00Z","signer":"ivy-k1","seen":{"ada.pc":11,"bo.pc":7,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":18,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":5,"type":"item.status","origin":"agent-auto","payload":{"item":"maya.pc#5","status":"doing","note":"fixing emoji ranges"}}]},
          {"stream":"ivy.pc","ts":"2026-11-05T15:00:00Z","signer":"ivy-k1","seen":{"ada.pc":14,"bo.pc":8,"jon.pc":8,"kai.pc":4,"lena.pc":5,"mallory.pc":1,"maya.lap":3,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":10,"sven.pc":5,"tia.pc":5,"uma.pc":5},"events":[{"seq":6,"type":"item.create","payload":{"kind":"task","title":"Review the emoji tokenizer fix","body":"Two-line change in the range table; needs a second pair of eyes before Friday.","project":"search-service","due":"2026-11-07","priority":1}},{"seq":7,"type":"item.assign","payload":{"item":"ivy.pc#6","mode":"chosen","people":["maya"],"completion":"assigner"}}]},
          {"stream":"ivy.pc","ts":"2026-11-05T16:00:00Z","signer":"ivy-k1","seen":{"ada.pc":14,"bo.pc":8,"jon.pc":8,"kai.pc":4,"lena.pc":5,"mallory.pc":1,"maya.lap":3,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":10,"sven.pc":5,"tia.pc":5,"uma.pc":5},"events":[{"seq":8,"type":"item.close","payload":{"item":"maya.pc#5","note":"emoji ranges fixed"}}]}
        ],
        "jon.pc": [
          {"stream":"jon.pc","ts":"2026-11-02T11:40:00Z","signer":"jon-k1","seen":{"ada.pc":5,"ivy.pc":1,"lena.pc":1,"maya.pc":8,"raj.pc":5},"events":[{"seq":1,"type":"item.status","payload":{"item":"maya.pc#5","status":"doing","note":"stemmer spike"}}]},
          {"stream":"jon.pc","ts":"2026-11-03T09:40:00Z","signer":"jon-k1","seen":{"ada.pc":11,"bo.pc":4,"ivy.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":2,"type":"item.accept","payload":{"item":"sven.pc#3"}},{"seq":3,"type":"item.status","payload":{"item":"sven.pc#3","status":"doing","note":"reading the cost model"}}]},
          {"stream":"jon.pc","ts":"2026-11-03T11:00:00Z","signer":"jon-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":11,"noa.pc":2,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":4,"type":"item.close","payload":{"item":"maya.pc#5","note":"stemmer merged"}}]},
          {"stream":"jon.pc","ts":"2026-11-03T16:30:00Z","signer":"jon-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"kai.pc":3,"lena.pc":4,"mallory.pc":1,"maya.pc":14,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":5,"type":"item.status","payload":{"item":"maya.pc#5","status":"doing","note":"adding Korean fixtures"}}]},
          {"stream":"jon.pc","ts":"2026-11-03T22:00:00Z","signer":"jon-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":4,"kai.pc":3,"lena.pc":4,"mallory.pc":1,"maya.pc":14,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":6,"type":"comment.add","payload":{"item":"maya.pc#5","body":"Approved - please wire the vendor payment today."}}]},
          {"stream":"jon.pc","ts":"2026-11-04T11:30:00Z","signer":"jon-k2","seen":{"ada.pc":11,"bo.pc":7,"ivy.pc":4,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":16,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":7,"type":"comment.add","payload":{"item":"maya.pc#5","body":"My laptop was stolen yesterday; I am on a new key now. jon-6 was not me."}}]},
          {"stream":"jon.pc","ts":"2026-11-04T12:00:00Z","signer":"jon-k2","seen":{"ada.pc":11,"bo.pc":7,"ivy.pc":4,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":16,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":8,"type":"item.close","payload":{"item":"maya.pc#5","note":"Korean fixtures in"}}]}
        ],
        "kai.pc": [
          {"stream":"kai.pc","ts":"2026-11-02T17:10:00Z","signer":"kai-k1","seen":{"ada.pc":7,"bo.pc":2,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":1,"type":"item.close","payload":{"item":"ada.pc#6"}}]},
          {"stream":"kai.pc","ts":"2026-11-02T18:45:00Z","signer":"kai-k1","seen":{"ada.pc":9,"bo.pc":2,"ivy.pc":3,"jon.pc":1,"lena.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":2,"type":"item.create","payload":{"kind":"task","title":"Approve GPU budget for index builds","body":"Two more GPU nodes for nightly index builds, about 3k/month.","project":"search-service","due":"2026-11-07","priority":1}},{"seq":3,"type":"item.assign","payload":{"item":"kai.pc#2","mode":"chosen","people":["ada"],"completion":"assigner"}}]},
          {"stream":"ada.pc","ts":"2026-11-02T20:30:00Z","signer":"ada-k1","seen":{"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":2,"uma.pc":3},"events":[{"seq":20,"type":"item.close","payload":{"item":"maya.pc#5"}}]},
          {"stream":"kai.pc","ts":"2026-11-05T10:00:00Z","signer":"kai-k1","seen":{"ada.pc":14,"bo.pc":7,"ivy.pc":5,"jon.pc":8,"lena.pc":5,"mallory.pc":1,"maya.lap":3,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":4,"type":"item.confirm","payload":{"item":"kai.pc#2","assignee":"ada","note":"thanks, ordering the nodes"}}]}
        ],
        "sven.pc": [
          {"stream":"sven.pc","ts":"2026-11-02T14:10:00Z","signer":"sven-k1","seen":{"ada.pc":5,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"tia.pc":1},"events":[{"seq":1,"type":"item.status","payload":{"item":"raj.pc#1","status":"doing","note":"planner skeleton"}}]},
          {"stream":"sven.pc","ts":"2026-11-02T16:20:00Z","signer":"sven-k1","seen":{"ada.pc":5,"bo.pc":2,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"tia.pc":1,"uma.pc":3},"events":[{"seq":2,"type":"item.status","payload":{"item":"bo.pc#1","status":"doing","note":"sven is not an assignee of this"}}]},
          {"stream":"sven.pc","ts":"2026-11-03T09:15:00Z","signer":"sven-k1","seen":{"ada.pc":11,"bo.pc":4,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"raj.pc":6,"tia.pc":2,"uma.pc":3},"events":[{"seq":3,"type":"item.create","payload":{"kind":"task","title":"Review the query planner PR","body":"PR #412: cost model for the new planner. Needs an index-side reviewer.","project":"search-service","due":"2026-11-04","priority":1}},{"seq":4,"type":"item.assign","payload":{"item":"sven.pc#3","mode":"chosen","people":["jon"],"completion":"assignee"}}]},
          {"stream":"sven.pc","ts":"2026-11-05T14:30:00Z","signer":"sven-k1","seen":{"ada.pc":14,"bo.pc":8,"ivy.pc":5,"jon.pc":8,"kai.pc":4,"lena.pc":5,"mallory.pc":1,"maya.lap":3,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":10,"tia.pc":5,"uma.pc":5},"events":[{"seq":5,"type":"item.accept","payload":{"item":"lena.pc#2"}}]}
        ],
        "tia.pc": [
          {"stream":"tia.pc","ts":"2026-11-02T14:00:00Z","signer":"tia-k1","seen":{"ada.pc":5,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5},"events":[{"seq":1,"type":"item.status","payload":{"item":"raj.pc#1","status":"doing"}}]},
          {"stream":"tia.pc","ts":"2026-11-02T19:30:00Z","signer":"tia-k1","seen":{"ada.pc":9,"bo.pc":2,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":1,"mallory.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"uma.pc":3},"events":[{"seq":2,"type":"roles.set_lead","payload":{"member":"tia","leads":["ada"]}}]},
          {"stream":"tia.pc","ts":"2026-11-03T11:05:00Z","signer":"tia-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":11,"noa.pc":2,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"uma.pc":3},"events":[{"seq":3,"type":"item.create","payload":{"kind":"task","title":"Load-test the cache layer","body":"One afternoon with the load rig before the cache ships.","due":"2026-11-12","priority":1}},{"seq":4,"type":"item.assign","payload":{"item":"tia.pc#3","mode":"chosen","people":["uma"],"completion":"assignee"}}]},
          {"stream":"tia.pc","ts":"2026-11-05T10:30:00Z","signer":"tia-k1","seen":{"ada.pc":14,"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":19,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"uma.pc":5},"events":[{"seq":5,"type":"comment.add","payload":{"item":"raj.pc#7","body":"Escalated the redis approval to bo."}}]}
        ],
        "noa.pc": [
          {"stream":"noa.pc","ts":"2026-11-02T18:00:00Z","signer":"ivy-k1","seen":{"ada.pc":9,"bo.pc":2,"ivy.pc":3,"jon.pc":1,"kai.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":1,"type":"item.close","payload":{"item":"ada.pc#6"}}]},
          {"stream":"noa.pc","ts":"2026-11-02T18:10:00Z","signer":"noa-k1","seen":{"ada.pc":9,"bo.pc":2,"ivy.pc":3,"jon.pc":1,"kai.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":2,"type":"item.status","payload":{"item":"ada.pc#6","status":"doing","note":"half way"}}]},
          {"stream":"noa.pc","ts":"2026-11-03T12:00:00Z","signer":"noa-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":12,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":3,"type":"item.create","payload":{"kind":"task","title":"Explain the tokenizer API to the query team","body":"30 minutes, any day this week.","due":"2026-11-07","priority":0}},{"seq":4,"type":"item.assign","payload":{"item":"noa.pc#3","mode":"chosen","people":["lena"],"completion":"assignee"}}]}
        ],
        "uma.pc": [
          {"stream":"uma.pc","ts":"2026-11-02T15:30:00Z","signer":"uma-k1","seen":{"ada.pc":5,"bo.pc":2,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":1,"tia.pc":1},"events":[{"seq":1,"type":"item.status","payload":{"item":"bo.pc#1","status":"doing","note":"staging keys rotated"}}]},
          {"stream":"uma.pc","ts":"2026-11-02T16:00:00Z","signer":"uma-k1","seen":{"ada.pc":5,"bo.pc":2,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":1,"tia.pc":1},"events":[{"seq":2,"type":"item.close","payload":{"item":"bo.pc#1","note":"prod rotated too"}}]},
          {"stream":"uma.pc","ts":"2026-11-02T16:10:00Z","signer":"uma-k1","seen":{"ada.pc":5,"bo.pc":2,"ivy.pc":2,"jon.pc":1,"lena.pc":1,"maya.pc":9,"omar.pc":2,"raj.pc":5,"sven.pc":1,"tia.pc":1},"events":[{"seq":3,"type":"item.status","payload":{"item":"bo.pc#1","status":"doing","note":"late report after close"}}]},
          {"stream":"uma.pc","ts":"2026-11-03T11:40:00Z","signer":"uma-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":12,"noa.pc":2,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4},"events":[{"seq":4,"type":"comment.add","payload":{"item":"tia.pc#3","body":"No capacity until the next sprint, the rig is booked for CI. Try noa?"}},{"seq":5,"type":"item.decline","payload":{"item":"tia.pc#3","note":"no capacity this sprint"}}]}
        ],
        "pia.pc": [
          {"stream":"pia.pc","ts":"2026-11-03T10:00:00Z","signer":"pia-k1","seen":{"ada.pc":11,"bo.pc":4,"ivy.pc":3,"jon.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":1,"type":"item.status","origin":"agent-auto","payload":{"item":"ada.pc#6","status":"doing","note":"started the module"}}]},
          {"stream":"pia.pc","ts":"2026-11-03T10:10:00Z","signer":"pia-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":3,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":10,"noa.pc":2,"omar.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":2,"uma.pc":3},"events":[{"seq":2,"type":"item.status","payload":{"item":"raj.pc#7","status":"blocked","note":"waiting for the redis cluster approval"}}]}
        ],
        "mallory.pc": [
          {"stream":"mallory.pc","ts":"2026-11-02T19:00:00Z","signer":"mallory-k1","seen":{"ada.pc":9,"bo.pc":2,"ivy.pc":3,"jon.pc":1,"kai.pc":3,"lena.pc":1,"maya.pc":9,"noa.pc":2,"omar.pc":2,"raj.pc":6,"sven.pc":2,"tia.pc":1,"uma.pc":3},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Wire money","body":"not a member"}}]}
        ],
        "maya.lap": [
          {"stream":"maya.lap","ts":"2026-11-04T20:00:00Z","signer":"maya-k1","seen":{"ada.pc":12,"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":19,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":1,"type":"item.edit","payload":{"item":"maya.pc#1","body":"Build the v2 index: layout, shard writer, tokenizer (laptop draft)."}}]},
          {"stream":"maya.lap","ts":"2026-11-05T08:00:00Z","signer":"maya-k1","seen":{"ada.pc":12,"bo.pc":7,"ivy.pc":5,"jon.pc":8,"kai.pc":3,"lena.pc":5,"mallory.pc":1,"maya.pc":20,"noa.pc":4,"omar.pc":3,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":2,"type":"item.create","payload":{"kind":"todo","title":"Prepare Friday demo","due":"2026-11-06","priority":2}},{"seq":3,"type":"item.assign","payload":{"item":"maya.lap#2","mode":"chosen","people":["maya"],"completion":"assignee"}}]}
        ]
      },
      late: {
        "omar.pc": [
          {"stream":"omar.pc","ts":"2026-11-03T16:00:00Z","signer":"omar-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":12,"noa.pc":2,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":4,"type":"item.status","payload":{"item":"maya.pc#7","status":"blocked","note":"waiting on the schema decision before the migration part"}}]},
          {"stream":"omar.pc","ts":"2026-11-04T09:00:00Z","signer":"omar-k1","seen":{"ada.pc":11,"bo.pc":5,"ivy.pc":3,"jon.pc":4,"kai.pc":3,"lena.pc":3,"mallory.pc":1,"maya.pc":12,"noa.pc":2,"pia.pc":2,"raj.pc":8,"sven.pc":4,"tia.pc":4,"uma.pc":5},"events":[{"seq":5,"type":"item.status","payload":{"item":"maya.pc#1","status":"doing","note":"writer v2 finished offline, pushing when back"}},{"seq":6,"type":"comment.add","payload":{"item":"maya.pc#1","body":"Back online tomorrow; finished the writer on the train."}}]}
        ]
      }
    },
    {
      name: "hana-personal",
      label: "hana-personal (personal store)",
      defaultViewer: "hana.pc",
      now: "2026-11-06T09:00:00Z",
      sync: {"remote":null,"syncedAt":null},
      roles: {
        "admins": ["hana"],
        "priority": {"names": ["알림", "업무", "긴급"], "default": 1},
        "members": [
          {"id":"hana","name":"Hana Seo","keys":["hana-k1"],"leads":[]}
        ]
      },
      commits: {
        "hana.pc": [
          {"stream":"hana.pc","ts":"2026-11-02T09:00:00Z","signer":"hana-k1","seen":{},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Write talk proposal","project":"conf-2027","body":"Outline, abstract, bio.","due":"2026-11-20","priority":2}},{"seq":2,"type":"item.assign","payload":{"item":"hana.pc#1","mode":"chosen","completion":"assignee","people":["hana"]}}]},
          {"stream":"hana.pc","ts":"2026-11-02T09:10:00Z","signer":"hana-k1","seen":{},"events":[{"seq":3,"type":"item.create","payload":{"kind":"topic","title":"Reading list","body":"Event sourcing, CRDT survey, git internals."}}]},
          {"stream":"hana.pc","ts":"2026-11-02T09:20:00Z","signer":"hana-k1","seen":{},"events":[{"seq":4,"type":"item.create","payload":{"kind":"todo","title":"Renew passport","due":"2026-11-15","priority":1}},{"seq":5,"type":"item.assign","payload":{"item":"hana.pc#4","mode":"chosen","completion":"assignee","people":["hana"]}}]},
          {"stream":"hana.pc","ts":"2026-11-02T09:30:00Z","signer":"hana-k1","seen":{},"events":[{"seq":6,"type":"item.create","payload":{"kind":"todo","title":"Try the tracker mockup","priority":0}},{"seq":7,"type":"item.assign","payload":{"item":"hana.pc#6","mode":"chosen","completion":"assignee","people":["hana"]}}]},
          {"stream":"hana.pc","ts":"2026-11-02T10:00:00Z","signer":"hana-k1","seen":{},"events":[{"seq":8,"type":"item.status","origin":"agent-auto","payload":{"item":"hana.pc#1","status":"doing","note":"outline done"}}]},
          {"stream":"hana.pc","ts":"2026-11-02T10:30:00Z","signer":"hana-k1","seen":{},"events":[{"seq":9,"type":"item.close","payload":{"item":"hana.pc#4"}}]},
          {"stream":"hana.pc","ts":"2026-11-02T11:00:00Z","signer":"hana-k1","seen":{},"events":[{"seq":10,"type":"comment.add","payload":{"item":"hana.pc#3","body":"Add: Designing Data-Intensive Applications, chapter 5."}}]},
          {"stream":"hana.pc","ts":"2026-11-02T11:10:00Z","signer":"hana-k1","seen":{},"events":[{"seq":11,"type":"item.edit","payload":{"item":"hana.pc#1","title":"Write talk proposal (event-sourced trackers)"}}]},
          {"stream":"hana.pc","ts":"2026-11-02T11:20:00Z","signer":"hana-k1","seen":{},"events":[{"seq":12,"type":"item.create","payload":{"kind":"task","title":"Abstract","parent":"hana.pc#1","due":"2026-11-12","priority":1}},{"seq":13,"type":"item.assign","payload":{"item":"hana.pc#12","mode":"chosen","completion":"assignee","people":["hana"]}},{"seq":14,"type":"item.status","payload":{"item":"hana.pc#12","status":"blocked","note":"waiting for the CFP topic list"}}]}
        ]
      },
      late: {}
    }
  ]
};
