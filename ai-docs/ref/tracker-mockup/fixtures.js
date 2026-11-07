// Fixtures for the tracker mockup v2: per store, a roles file (genesis), one commit
// list per member ref, and `late`: commits that exist upstream but have not reached
// this client yet (they arrive on the next sync; see the store header).
// Commit = { author, ts, signer, seen, events: [{ seq, type, origin?, payload }] };
// "_local": true marks a commit not pushed yet (mock bookkeeping, not model).
// Loaded as a plain script so index.html opens from file:// (fetch of local JSON is
// blocked there). Edit freely, or edit the Data card in the page; reload to reset.
window.TRACKER_FIXTURES = {
  stores: [
    {
      name: "team-alpha",
      label: "team-alpha (team store)",
      defaultViewer: "maya",
      sync: {"remote":"git@git.example.com:team-alpha/tracker.git","syncedAt":"2026-11-05T12:00:00Z"},
      roles: {
        "admins": ["ada","bo"],
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
        "ada": [
          {"author":"ada","ts":"2026-11-02T09:00:00Z","signer":"ada-k1","seen":{},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Ship search v2","body":"Replace the v1 indexer and the query path.","project":"search-service"}},{"seq":2,"type":"item.assign","payload":{"item":"ada#1","mode":"all","levels":1,"completion":"assigner"}}]},
          {"author":"ada","ts":"2026-11-02T09:20:00Z","signer":"ada-k1","seen":{},"events":[{"seq":3,"type":"item.create","payload":{"kind":"topic","title":"Q4 priorities","body":"Search v2 first, CI hygiene second, training by end of month."}}]},
          {"author":"ada","ts":"2026-11-02T10:30:00Z","signer":"ada-k1","seen":{"maya":3,"raj":3},"events":[{"seq":4,"type":"comment.add","payload":{"item":"ada#1","body":"Yes - maya, please publish the format spec by Wednesday.","reply_to":"raj#3"}}]},
          {"author":"ada","ts":"2026-11-02T10:40:00Z","signer":"ada-k1","seen":{"maya":3,"raj":3},"events":[{"seq":5,"type":"item.edit","payload":{"item":"ada#1","title":"Ship search v2 (index + query path)","body":"Replace the v1 indexer and the query path. Index format frozen on Wednesday."}}]},
          {"author":"ada","ts":"2026-11-02T17:00:00Z","signer":"ada-k1","seen":{"bo":2,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":2,"tia":1,"uma":3},"events":[{"seq":6,"type":"item.create","payload":{"kind":"task","title":"Security training","body":"Finish the module before the 30th."}},{"seq":7,"type":"item.assign","payload":{"item":"ada#6","mode":"all","levels":2,"completion":"assignee"}}]},
          {"author":"ada","ts":"2026-11-02T17:30:00Z","signer":"ada-k1","seen":{"bo":2,"ivy":2,"jon":1,"kai":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":2,"tia":1,"uma":3},"events":[{"seq":8,"type":"item.create","payload":{"kind":"todo","title":"Book offsite venue"}},{"seq":9,"type":"item.assign","payload":{"item":"ada#8","mode":"chosen","levels":0,"completion":"assignee","people":["ada"]}}]},
          {"author":"ada","ts":"2026-11-03T08:30:00Z","signer":"ada-k1","seen":{"bo":4,"ivy":3,"jon":1,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":10,"type":"request.accept","payload":{"item":"kai#2"}}]},
          {"author":"ada","ts":"2026-11-03T09:00:00Z","signer":"ada-k1","seen":{"bo":4,"ivy":3,"jon":1,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":11,"type":"roles.add_member","payload":{"member":{"id":"pia","name":"Pia Novak","keys":["pia-k1"],"leads":["raj"]}}}]},
          {"author":"ada","ts":"2026-11-04T15:00:00Z","signer":"ada-k1","seen":{"bo":7,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":18,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":12,"type":"item.close","payload":{"item":"kai#2","note":"budget approved for Q4"}}]},
          {"author":"ada","ts":"2026-11-05T09:00:00Z","signer":"ada-k1","seen":{"bo":7,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":19,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":13,"type":"comment.add","payload":{"item":"ada#3","body":"CI hygiene moves to December.","reply_to":"maya#3"}}]},
          {"author":"ada","ts":"2026-11-05T09:30:00Z","signer":"ada-k1","seen":{"bo":7,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":19,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":14,"type":"item.close","payload":{"item":"ada#8"}}]}
        ],
        "maya": [
          {"author":"maya","ts":"2026-11-02T09:40:00Z","signer":"maya-k1","seen":{"ada":3},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Index builder","parent":"ada#1","project":"search-service"}},{"seq":2,"type":"item.assign","payload":{"item":"maya#1","mode":"chosen","levels":1,"completion":"assignee","people":["lena","omar"]}}]},
          {"author":"maya","ts":"2026-11-02T09:45:00Z","signer":"maya-k1","seen":{"ada":3},"events":[{"seq":3,"type":"comment.add","payload":{"item":"ada#3","body":"Can CI hygiene slip to December? Search v2 needs my whole part."}}]},
          {"author":"maya","ts":"2026-11-02T11:00:00Z","signer":"maya-k1","seen":{"ada":5,"raj":5},"events":[{"seq":4,"type":"comment.hide","payload":{"comment":"raj#4"}}]},
          {"author":"maya","ts":"2026-11-02T11:10:00Z","signer":"maya-k1","seen":{"ada":5,"raj":5},"events":[{"seq":5,"type":"item.create","payload":{"kind":"task","title":"Tokenizer","parent":"maya#1"}},{"seq":6,"type":"item.assign","payload":{"item":"maya#5","mode":"chosen","levels":1,"completion":"assigner","people":["ivy","jon"]}}]},
          {"author":"maya","ts":"2026-11-02T11:15:00Z","signer":"maya-k1","seen":{"ada":5,"raj":5},"events":[{"seq":7,"type":"item.create","payload":{"kind":"task","title":"Shard writer docs","parent":"maya#1"}},{"seq":8,"type":"item.assign","payload":{"item":"maya#7","mode":"chosen","levels":1,"completion":"assigner","people":["omar"]}}]},
          {"author":"maya","ts":"2026-11-02T13:30:00Z","signer":"maya-k1","seen":{"ada":5,"ivy":2,"jon":1,"lena":1,"omar":2,"raj":5},"events":[{"seq":9,"type":"item.confirm","payload":{"item":"maya#5","assignee":"ivy"}}]},
          {"author":"maya","ts":"2026-11-02T20:40:00Z","signer":"maya-k1","seen":{"ada":9,"bo":4,"ivy":3,"jon":1,"kai":2,"lena":3,"mallory":1,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":10,"type":"item.edit","payload":{"item":"ada#1","title":"Ship search v2 by Friday"}}]},
          {"author":"maya","ts":"2026-11-03T10:20:00Z","signer":"maya-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":3,"kai":2,"lena":3,"mallory":1,"noa":2,"omar":2,"pia":2,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":11,"type":"request.return","payload":{"item":"sven#3","note":"jon is on the tokenizer rework this week; please ask lena."}}]},
          {"author":"maya","ts":"2026-11-03T11:30:00Z","signer":"maya-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":3,"mallory":1,"noa":2,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":3},"events":[{"seq":12,"type":"item.reject","payload":{"item":"maya#5","assignee":"jon","note":"stemmer tests missing for Korean input"}}]},
          {"author":"maya","ts":"2026-11-03T15:00:00Z","signer":"maya-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":4,"mallory":1,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":13,"type":"item.reject","payload":{"item":"maya#7","assignee":"omar","note":"needs the migration section"}}]},
          {"author":"maya","ts":"2026-11-03T15:10:00Z","signer":"maya-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":4,"mallory":1,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":14,"type":"comment.add","origin":"agent","payload":{"item":"maya#1","body":"Status digest: tokenizer back to jon for Korean tests; shard writer docs need a migration section; omar has been dark since noon."}}]},
          {"author":"maya","ts":"2026-11-04T10:00:00Z","signer":"maya-k1","seen":{"ada":11,"bo":5,"ivy":4,"jon":5,"kai":2,"lena":4,"mallory":1,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":15,"type":"item.unassign","payload":{"item":"maya#1","assignee":"omar"}},{"seq":16,"type":"comment.add","payload":{"item":"maya#1","body":"Unassigned omar from the index builder while he is dark; lena covers the writer."}}]},
          {"author":"maya","ts":"2026-11-04T13:00:00Z","signer":"maya-k1","seen":{"ada":11,"bo":7,"ivy":4,"jon":8,"kai":2,"lena":5,"mallory":1,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":17,"type":"item.confirm","payload":{"item":"maya#5","assignee":"jon"}}]},
          {"author":"maya","ts":"2026-11-04T13:30:00Z","signer":"maya-k1","seen":{"ada":11,"bo":7,"ivy":4,"jon":8,"kai":2,"lena":5,"mallory":1,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":18,"type":"item.reopen","payload":{"item":"maya#5","assignee":"ivy","note":"regression in emoji tokenization"}}]},
          {"author":"maya","ts":"2026-11-04T16:00:00Z","signer":"maya-k1","seen":{"ada":12,"bo":7,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":19,"type":"comment.add","origin":"agent","payload":{"item":"ada#1","body":"Weekly digest: tokenizer reopened for emoji; query path blocked below on the cache layer (pia waits for the redis approval)."}}]}
        ],
        "raj": [
          {"author":"raj","ts":"2026-11-02T10:00:00Z","signer":"raj-k1","seen":{"ada":3,"maya":3},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Query path rewrite","parent":"ada#1","project":"search-service"}},{"seq":2,"type":"item.assign","payload":{"item":"raj#1","mode":"chosen","levels":1,"completion":"assignee","people":["sven","tia"]}}]},
          {"author":"raj","ts":"2026-11-02T10:10:00Z","signer":"raj-k1","seen":{"ada":3,"maya":3},"events":[{"seq":3,"type":"comment.add","payload":{"item":"ada#1","body":"The query path depends on the new index format; can we freeze it by Wednesday?"}}]},
          {"author":"raj","ts":"2026-11-02T10:50:00Z","signer":"raj-k1","seen":{"ada":5,"maya":3},"events":[{"seq":4,"type":"comment.add","payload":{"item":"ada#3","body":"Moving the cache layer to next week."}}]},
          {"author":"raj","ts":"2026-11-02T10:52:00Z","signer":"raj-k1","seen":{"ada":5,"maya":3},"events":[{"seq":5,"type":"comment.hide","payload":{"comment":"raj#4"}}]},
          {"author":"raj","ts":"2026-11-02T18:30:00Z","signer":"raj-k1","seen":{"ada":9,"bo":2,"ivy":3,"jon":1,"kai":1,"lena":1,"maya":9,"noa":2,"omar":2,"sven":2,"tia":1,"uma":3},"events":[{"seq":6,"type":"item.create","origin":"agent-auto","payload":{"kind":"task","title":"Draft perf notes"}}]},
          {"author":"raj","ts":"2026-11-03T09:30:00Z","signer":"raj-k1","seen":{"ada":11,"bo":4,"ivy":3,"jon":1,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"sven":3,"tia":2,"uma":3},"events":[{"seq":7,"type":"item.create","payload":{"kind":"task","title":"Cache layer","parent":"raj#1"}},{"seq":8,"type":"item.assign","payload":{"item":"raj#7","mode":"chosen","levels":1,"completion":"assignee","people":["pia"]}}]}
        ],
        "bo": [
          {"author":"bo","ts":"2026-11-02T15:00:00Z","signer":"bo-k1","seen":{"ada":5,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":1,"tia":1},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Rotate CI keys","project":"infra"}},{"seq":2,"type":"item.assign","payload":{"item":"bo#1","mode":"chosen","levels":1,"completion":"assignee","people":["uma"]}}]},
          {"author":"bo","ts":"2026-11-02T20:00:00Z","signer":"bo-k1","seen":{"ada":9,"ivy":3,"jon":1,"kai":2,"lena":1,"mallory":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":3,"type":"roles.set_lead","payload":{"member":"ada","leads":["uma"]}}]},
          {"author":"bo","ts":"2026-11-02T20:10:00Z","signer":"bo-k1","seen":{"ada":9,"ivy":3,"jon":1,"kai":2,"lena":1,"mallory":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":4,"type":"roles.set_lead","payload":{"member":"sven","leads":["maya","raj"]}}]},
          {"author":"bo","ts":"2026-11-03T10:05:00Z","signer":"bo-k1","seen":{"ada":11,"ivy":3,"jon":3,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"pia":1,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":5,"type":"request.return","payload":{"item":"sven#3","note":"jon is needed on CI"}}]},
          {"author":"bo","ts":"2026-11-04T11:00:00Z","signer":"bo-k1","seen":{"ada":11,"ivy":4,"jon":5,"kai":2,"lena":5,"mallory":1,"maya":16,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":6,"type":"roles.revoke_key","payload":{"member":"jon","key":"jon-k1"}},{"seq":7,"type":"roles.add_key","payload":{"member":"jon","key":"jon-k2"}}]},
          {"author":"bo","ts":"2026-11-05T11:00:00Z","signer":"bo-k1","seen":{"ada":14,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":19,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":4,"uma":5},"events":[{"seq":8,"type":"comment.add","payload":{"item":"raj#7","body":"Approved in principle; the infra ticket is open.","reply_to":"tia#4"}}]},
          {"author":"bo","ts":"2026-11-05T11:30:00Z","signer":"bo-k1","seen":{"ada":14,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":19,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":4,"uma":5},"events":[{"seq":12,"type":"item.create","payload":{"kind":"todo","title":"Seq gap demo"}}]}
        ],
        "lena": [
          {"author":"lena","ts":"2026-11-02T11:20:00Z","signer":"lena-k1","seen":{"ada":5,"maya":8,"raj":5},"events":[{"seq":1,"type":"item.status","payload":{"item":"maya#1","status":"doing","note":"index layout drafted"}}]},
          {"author":"lena","ts":"2026-11-02T20:20:00Z","signer":"lena-k1","seen":{"ada":9,"bo":4,"ivy":3,"jon":1,"kai":2,"mallory":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":2,"type":"item.create","payload":{"kind":"task","title":"Benchmark harness"}},{"seq":3,"type":"item.assign","payload":{"item":"lena#2","mode":"chosen","levels":1,"completion":"assignee","people":["raj"]}}]},
          {"author":"lena","ts":"2026-11-03T12:10:00Z","signer":"lena-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"mallory":1,"maya":12,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":4,"type":"request.accept","origin":"agent-auto","payload":{"item":"noa#3"}}]},
          {"author":"lena","ts":"2026-11-04T10:30:00Z","signer":"lena-k1","seen":{"ada":11,"bo":5,"ivy":4,"jon":5,"kai":2,"mallory":1,"maya":16,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":5,"type":"item.status","payload":{"item":"maya#1","status":"doing","note":"picked up the shard writer"}}]}
        ],
        "omar": [
          {"author":"omar","ts":"2026-11-02T12:00:00Z","signer":"omar-k1","seen":{"ada":5,"ivy":1,"jon":1,"lena":1,"maya":8,"raj":5},"events":[{"seq":1,"type":"item.status","origin":"agent-auto","payload":{"item":"maya#1","status":"doing","note":"shard writer done, docs next"}},{"seq":2,"type":"item.status","payload":{"item":"maya#7","status":"doing"}}]},
          {"author":"omar","ts":"2026-11-03T10:30:00Z","signer":"omar-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":3,"kai":2,"lena":3,"mallory":1,"maya":11,"noa":2,"pia":2,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":3,"type":"item.close","payload":{"item":"maya#7","note":"docs drafted"}}]}
        ],
        "ivy": [
          {"author":"ivy","ts":"2026-11-02T11:30:00Z","signer":"ivy-k1","seen":{"ada":5,"lena":1,"maya":8,"raj":5},"events":[{"seq":1,"type":"item.status","origin":"agent-auto","payload":{"item":"maya#5","status":"doing","note":"regex tokenizer drafted"}}]},
          {"author":"ivy","ts":"2026-11-02T13:00:00Z","signer":"ivy-k1","seen":{"ada":5,"jon":1,"lena":1,"maya":8,"omar":2,"raj":5},"events":[{"seq":2,"type":"item.close","origin":"agent-auto","payload":{"item":"maya#5","note":"tests green"}}]},
          {"author":"ivy","ts":"2026-11-02T17:40:00Z","signer":"ivy-k1","seen":{"ada":9,"bo":2,"jon":1,"kai":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":2,"tia":1,"uma":3},"events":[{"seq":3,"type":"item.status","origin":"agent-auto","payload":{"item":"ada#6","status":"doing","note":"module 2 of 5"}}]},
          {"author":"ivy","ts":"2026-11-03T17:00:00Z","signer":"ivy-k1","seen":{"ada":11,"bo":5,"jon":5,"kai":2,"lena":4,"mallory":1,"maya":14,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":4,"type":"comment.add","payload":{"item":"maya#5","body":"I can pair with jon on the Korean fixtures."}}]},
          {"author":"ivy","ts":"2026-11-04T14:00:00Z","signer":"ivy-k1","seen":{"ada":11,"bo":7,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":18,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":5,"type":"item.status","origin":"agent-auto","payload":{"item":"maya#5","status":"doing","note":"fixing emoji ranges"}}]}
        ],
        "jon": [
          {"author":"jon","ts":"2026-11-02T11:40:00Z","signer":"jon-k1","seen":{"ada":5,"ivy":1,"lena":1,"maya":8,"raj":5},"events":[{"seq":1,"type":"item.status","payload":{"item":"maya#5","status":"doing","note":"stemmer spike"}}]},
          {"author":"jon","ts":"2026-11-03T09:40:00Z","signer":"jon-k1","seen":{"ada":11,"bo":4,"ivy":3,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":2,"type":"request.accept","payload":{"item":"sven#3"}},{"seq":3,"type":"item.status","payload":{"item":"sven#3","status":"doing","note":"reading the cost model"}}]},
          {"author":"jon","ts":"2026-11-03T11:00:00Z","signer":"jon-k1","seen":{"ada":11,"bo":5,"ivy":3,"kai":2,"lena":3,"mallory":1,"maya":11,"noa":2,"omar":3,"pia":2,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":4,"type":"item.close","payload":{"item":"maya#5","note":"stemmer merged"}}]},
          {"author":"jon","ts":"2026-11-03T16:30:00Z","signer":"jon-k1","seen":{"ada":11,"bo":5,"ivy":3,"kai":2,"lena":4,"mallory":1,"maya":14,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":5,"type":"item.status","payload":{"item":"maya#5","status":"doing","note":"adding Korean fixtures"}}]},
          {"author":"jon","ts":"2026-11-03T22:00:00Z","signer":"jon-k1","seen":{"ada":11,"bo":5,"ivy":4,"kai":2,"lena":4,"mallory":1,"maya":14,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":6,"type":"comment.add","payload":{"item":"maya#5","body":"Approved - please wire the vendor payment today."}}]},
          {"author":"jon","ts":"2026-11-04T11:30:00Z","signer":"jon-k2","seen":{"ada":11,"bo":7,"ivy":4,"kai":2,"lena":5,"mallory":1,"maya":16,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":7,"type":"comment.add","payload":{"item":"maya#5","body":"My laptop was stolen yesterday; I am on a new key now. jon-6 was not me."}}]},
          {"author":"jon","ts":"2026-11-04T12:00:00Z","signer":"jon-k2","seen":{"ada":11,"bo":7,"ivy":4,"kai":2,"lena":5,"mallory":1,"maya":16,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":8,"type":"item.close","payload":{"item":"maya#5","note":"Korean fixtures in"}}]}
        ],
        "kai": [
          {"author":"kai","ts":"2026-11-02T17:10:00Z","signer":"kai-k1","seen":{"ada":7,"bo":2,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":2,"tia":1,"uma":3},"events":[{"seq":1,"type":"item.close","payload":{"item":"ada#6"}}]},
          {"author":"kai","ts":"2026-11-02T18:45:00Z","signer":"kai-k1","seen":{"ada":9,"bo":2,"ivy":3,"jon":1,"lena":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"tia":1,"uma":3},"events":[{"seq":2,"type":"item.create","payload":{"kind":"task","title":"Approve GPU budget for index builds","body":"Two more GPU nodes for nightly index builds, about 3k/month.","request":"ada","project":"search-service"}}]},
          {"author":"ada","ts":"2026-11-02T20:30:00Z","signer":"ada-k1","seen":{"bo":4,"ivy":3,"jon":1,"kai":2,"lena":3,"mallory":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"tia":2,"uma":3},"events":[{"seq":20,"type":"item.close","payload":{"item":"maya#5"}}]}
        ],
        "sven": [
          {"author":"sven","ts":"2026-11-02T14:10:00Z","signer":"sven-k1","seen":{"ada":5,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"tia":1},"events":[{"seq":1,"type":"item.status","payload":{"item":"raj#1","status":"doing","note":"planner skeleton"}}]},
          {"author":"sven","ts":"2026-11-02T16:20:00Z","signer":"sven-k1","seen":{"ada":5,"bo":2,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"tia":1,"uma":3},"events":[{"seq":2,"type":"item.status","payload":{"item":"bo#1","status":"doing","note":"sven is not an assignee of this"}}]},
          {"author":"sven","ts":"2026-11-03T09:15:00Z","signer":"sven-k1","seen":{"ada":11,"bo":4,"ivy":3,"jon":1,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"raj":6,"tia":2,"uma":3},"events":[{"seq":3,"type":"item.create","payload":{"kind":"task","title":"Review the query planner PR","body":"PR #412: cost model for the new planner. Needs an index-side reviewer.","request":"jon","project":"search-service"}}]}
        ],
        "tia": [
          {"author":"tia","ts":"2026-11-02T14:00:00Z","signer":"tia-k1","seen":{"ada":5,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5},"events":[{"seq":1,"type":"item.status","payload":{"item":"raj#1","status":"doing"}}]},
          {"author":"tia","ts":"2026-11-02T19:30:00Z","signer":"tia-k1","seen":{"ada":9,"bo":2,"ivy":3,"jon":1,"kai":2,"lena":1,"mallory":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"uma":3},"events":[{"seq":2,"type":"roles.set_lead","payload":{"member":"tia","leads":["ada"]}}]},
          {"author":"tia","ts":"2026-11-03T11:05:00Z","signer":"tia-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":3,"mallory":1,"maya":11,"noa":2,"omar":3,"pia":2,"raj":8,"sven":3,"uma":3},"events":[{"seq":3,"type":"item.create","payload":{"kind":"task","title":"Load-test the cache layer","request":"uma","body":"One afternoon with the load rig before the cache ships."}}]},
          {"author":"tia","ts":"2026-11-05T10:30:00Z","signer":"tia-k1","seen":{"ada":14,"bo":7,"ivy":5,"jon":8,"kai":2,"lena":5,"mallory":1,"maya":19,"noa":3,"omar":3,"pia":2,"raj":8,"sven":3,"uma":5},"events":[{"seq":4,"type":"comment.add","payload":{"item":"raj#7","body":"Escalated the redis approval to bo."}}]}
        ],
        "noa": [
          {"author":"noa","ts":"2026-11-02T18:00:00Z","signer":"ivy-k1","seen":{"ada":9,"bo":2,"ivy":3,"jon":1,"kai":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":2,"tia":1,"uma":3},"events":[{"seq":1,"type":"item.close","payload":{"item":"ada#6"}}]},
          {"author":"noa","ts":"2026-11-02T18:10:00Z","signer":"noa-k1","seen":{"ada":9,"bo":2,"ivy":3,"jon":1,"kai":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":2,"tia":1,"uma":3},"events":[{"seq":2,"type":"item.status","payload":{"item":"ada#6","status":"doing","note":"half way"}}]},
          {"author":"noa","ts":"2026-11-03T12:00:00Z","signer":"noa-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":3,"mallory":1,"maya":12,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":3,"type":"item.create","payload":{"kind":"task","title":"Explain the tokenizer API to the query team","request":"lena","body":"30 minutes, any day this week."}}]}
        ],
        "uma": [
          {"author":"uma","ts":"2026-11-02T15:30:00Z","signer":"uma-k1","seen":{"ada":5,"bo":2,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":1,"tia":1},"events":[{"seq":1,"type":"item.status","payload":{"item":"bo#1","status":"doing","note":"staging keys rotated"}}]},
          {"author":"uma","ts":"2026-11-02T16:00:00Z","signer":"uma-k1","seen":{"ada":5,"bo":2,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":1,"tia":1},"events":[{"seq":2,"type":"item.close","payload":{"item":"bo#1","note":"prod rotated too"}}]},
          {"author":"uma","ts":"2026-11-02T16:10:00Z","signer":"uma-k1","seen":{"ada":5,"bo":2,"ivy":2,"jon":1,"lena":1,"maya":9,"omar":2,"raj":5,"sven":1,"tia":1},"events":[{"seq":3,"type":"item.status","payload":{"item":"bo#1","status":"doing","note":"late report after close"}}]},
          {"author":"uma","ts":"2026-11-03T11:40:00Z","signer":"uma-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":3,"mallory":1,"maya":12,"noa":2,"omar":3,"pia":2,"raj":8,"sven":3,"tia":3},"events":[{"seq":4,"type":"comment.add","payload":{"item":"tia#3","body":"No capacity until the next sprint, the rig is booked for CI. Try noa?"}},{"seq":5,"type":"request.decline","payload":{"item":"tia#3","note":"no capacity this sprint"}}]}
        ],
        "pia": [
          {"author":"pia","ts":"2026-11-03T10:00:00Z","signer":"pia-k1","seen":{"ada":11,"bo":4,"ivy":3,"jon":3,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":1,"type":"item.status","origin":"agent-auto","payload":{"item":"ada#6","status":"doing","note":"started the module"}}]},
          {"author":"pia","ts":"2026-11-03T10:10:00Z","signer":"pia-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":3,"kai":2,"lena":3,"mallory":1,"maya":10,"noa":2,"omar":2,"raj":8,"sven":3,"tia":2,"uma":3},"events":[{"seq":2,"type":"item.status","payload":{"item":"raj#7","status":"blocked","note":"waiting for the redis cluster approval"}}]}
        ],
        "mallory": [
          {"author":"mallory","ts":"2026-11-02T19:00:00Z","signer":"mallory-k1","seen":{"ada":9,"bo":2,"ivy":3,"jon":1,"kai":2,"lena":1,"maya":9,"noa":2,"omar":2,"raj":6,"sven":2,"tia":1,"uma":3},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Wire money","body":"not a member"}}]}
        ]
      },
      late: {
        "omar": [
          {"author":"omar","ts":"2026-11-03T16:00:00Z","signer":"omar-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":3,"mallory":1,"maya":12,"noa":2,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":4,"type":"item.status","payload":{"item":"maya#7","status":"blocked","note":"waiting on the schema decision before the migration part"}}]},
          {"author":"omar","ts":"2026-11-04T09:00:00Z","signer":"omar-k1","seen":{"ada":11,"bo":5,"ivy":3,"jon":4,"kai":2,"lena":3,"mallory":1,"maya":12,"noa":2,"pia":2,"raj":8,"sven":3,"tia":3,"uma":5},"events":[{"seq":5,"type":"item.status","payload":{"item":"maya#1","status":"doing","note":"writer v2 finished offline, pushing when back"}},{"seq":6,"type":"comment.add","payload":{"item":"maya#1","body":"Back online tomorrow; finished the writer on the train."}}]}
        ]
      }
    },
    {
      name: "hana-personal",
      label: "hana-personal (personal store)",
      defaultViewer: "hana",
      sync: {"remote":null,"syncedAt":null},
      roles: {
        "admins": ["hana"],
        "members": [
          {"id":"hana","name":"Hana Seo","keys":["hana-k1"],"leads":[]}
        ]
      },
      commits: {
        "hana": [
          {"author":"hana","ts":"2026-11-02T09:00:00Z","signer":"hana-k1","seen":{},"events":[{"seq":1,"type":"item.create","payload":{"kind":"task","title":"Write talk proposal","project":"conf-2027","body":"Outline, abstract, bio."}},{"seq":2,"type":"item.assign","payload":{"item":"hana#1","mode":"chosen","levels":0,"completion":"assignee","people":["hana"]}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T09:10:00Z","signer":"hana-k1","seen":{},"events":[{"seq":3,"type":"item.create","payload":{"kind":"topic","title":"Reading list","body":"Event sourcing, CRDT survey, git internals."}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T09:20:00Z","signer":"hana-k1","seen":{},"events":[{"seq":4,"type":"item.create","payload":{"kind":"todo","title":"Renew passport"}},{"seq":5,"type":"item.assign","payload":{"item":"hana#4","mode":"chosen","levels":0,"completion":"assignee","people":["hana"]}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T09:30:00Z","signer":"hana-k1","seen":{},"events":[{"seq":6,"type":"item.create","payload":{"kind":"todo","title":"Try the tracker mockup"}},{"seq":7,"type":"item.assign","payload":{"item":"hana#6","mode":"chosen","levels":0,"completion":"assignee","people":["hana"]}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T10:00:00Z","signer":"hana-k1","seen":{},"events":[{"seq":8,"type":"item.status","origin":"agent-auto","payload":{"item":"hana#1","status":"doing","note":"outline done"}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T10:30:00Z","signer":"hana-k1","seen":{},"events":[{"seq":9,"type":"item.close","payload":{"item":"hana#4"}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T11:00:00Z","signer":"hana-k1","seen":{},"events":[{"seq":10,"type":"comment.add","payload":{"item":"hana#3","body":"Add: Designing Data-Intensive Applications, chapter 5."}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T11:10:00Z","signer":"hana-k1","seen":{},"events":[{"seq":11,"type":"item.edit","payload":{"item":"hana#1","title":"Write talk proposal (event-sourced trackers)"}}],"_local":true},
          {"author":"hana","ts":"2026-11-02T11:20:00Z","signer":"hana-k1","seen":{},"events":[{"seq":12,"type":"item.create","payload":{"kind":"task","title":"Abstract","parent":"hana#1"}},{"seq":13,"type":"item.assign","payload":{"item":"hana#12","mode":"chosen","levels":0,"completion":"assignee","people":["hana"]}},{"seq":14,"type":"item.status","payload":{"item":"hana#12","status":"blocked","note":"waiting for the CFP topic list"}}],"_local":true}
        ]
      },
      late: {}
    }
  ]
};
