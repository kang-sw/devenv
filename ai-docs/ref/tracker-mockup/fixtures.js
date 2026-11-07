// Fixtures for the tracker mockup: a roles file and one event log per member ref, per store.
// Loaded as a plain script so index.html opens from file:// (fetch of local JSON is blocked there).
// Edit freely, or edit the raw pane in the page; reload the page to return to these.
window.TRACKER_FIXTURES = {
  stores: [
    {
      name: "team-alpha",
      label: "team-alpha (team store)",
      defaultViewer: "lena",
      roles: {
        "admins": ["ada","bo"],
        "members": [
          {"id":"ada","name":"Ada Park","keys":["ada-k1"],"leads":[]},
          {"id":"maya","name":"Maya Ortiz","keys":["maya-k1"],"leads":["ada"]},
          {"id":"raj","name":"Raj Rao","keys":["raj-k1"],"leads":["ada"]},
          {"id":"bo","name":"Bo Lindqvist","keys":["bo-k1"],"leads":["ada"]},
          {"id":"lena","name":"Lena Weiss","keys":["lena-k1"],"leads":["maya"]},
          {"id":"omar","name":"Omar Haddad","keys":["omar-k1"],"leads":["maya"]},
          {"id":"sven","name":"Sven Berg","keys":["sven-k1"],"leads":["raj"]},
          {"id":"tia","name":"Tia Moreau","keys":["tia-k1"],"leads":["raj"]},
          {"id":"uma","name":"Uma Iyer","keys":["uma-k1"],"leads":["bo"]},
          {"id":"ivy","name":"Ivy Chen","keys":["ivy-k1"],"leads":["lena"]},
          {"id":"jon","name":"Jon Ek","keys":["jon-k1"],"leads":["lena"]},
          {"id":"kai","name":"Kai Mori","keys":["kai-k1"],"leads":["omar"]},
          {"id":"noa","name":"Noa Fisher","keys":["noa-k1"],"leads":["sven"]}
        ]
      },
      logs: {
        "ada": [
          {"author":"ada","seq":1,"ts":"2026-11-02T09:17:00Z","sig":"ada-k1","type":"item.create","payload":{"kind":"task","title":"Ship search v2","body":"Replace the v1 indexer and the query path.","project":"search-service"}},
          {"author":"ada","seq":2,"ts":"2026-11-02T09:34:00Z","sig":"ada-k1","type":"item.assign","payload":{"item":"ada#1","mode":"all","levels":1,"completion":"assigner"}},
          {"author":"ada","seq":3,"ts":"2026-11-02T09:51:00Z","sig":"ada-k1","type":"item.create","payload":{"kind":"note","title":"Q4 priorities","body":"Search v2 first, CI hygiene second, training by end of month."}},
          {"author":"ada","seq":4,"ts":"2026-11-02T10:08:00Z","sig":"ada-k1","type":"roles.add_member","payload":{"member":{"id":"pia","name":"Pia Novak","keys":["pia-k1"],"leads":["tia"]}}},
          {"author":"ada","seq":5,"ts":"2026-11-02T17:13:00Z","sig":"ada-k1","type":"item.create","payload":{"kind":"task","title":"Security training","body":"Finish the module before the 30th."}},
          {"author":"ada","seq":6,"ts":"2026-11-02T17:30:00Z","sig":"ada-k1","type":"item.assign","payload":{"item":"ada#5","mode":"all","levels":3,"completion":"assignee"}},
          {"author":"ada","seq":7,"ts":"2026-11-02T21:45:00Z","sig":"ada-k1","type":"item.create","payload":{"kind":"todo","title":"Book offsite venue"}}
        ],
        "mallory": [
          {"author":"mallory","seq":1,"ts":"2026-11-02T10:25:00Z","sig":"mallory-k1","type":"item.create","payload":{"kind":"task","title":"Wire money","body":"not a member"}}
        ],
        "maya": [
          {"author":"maya","seq":1,"ts":"2026-11-02T10:42:00Z","sig":"maya-k1","type":"item.create","payload":{"kind":"task","title":"Index builder","parent":"ada#1"}},
          {"author":"maya","seq":2,"ts":"2026-11-02T10:59:00Z","sig":"maya-k1","type":"item.assign","payload":{"item":"maya#1","mode":"chosen","levels":1,"people":["lena","omar"],"completion":"assignee"}},
          {"author":"maya","seq":3,"ts":"2026-11-02T18:55:00Z","sig":"maya-k1","type":"item.create","payload":{"kind":"task","title":"Perf budget"}},
          {"author":"maya","seq":4,"ts":"2026-11-02T19:12:00Z","sig":"maya-k1","type":"item.assign","payload":{"item":"maya#3","mode":"chosen","levels":1,"people":["ivy"],"completion":"assignee"}}
        ],
        "lena": [
          {"author":"lena","seq":1,"ts":"2026-11-02T11:16:00Z","sig":"lena-k1","type":"item.create","payload":{"kind":"task","title":"Tokenizer","parent":"maya#1"}},
          {"author":"lena","seq":2,"ts":"2026-11-02T11:33:00Z","sig":"lena-k1","type":"item.assign","payload":{"item":"lena#1","mode":"chosen","levels":1,"people":["ivy","jon"],"completion":"assigner"}},
          {"author":"lena","seq":3,"ts":"2026-11-02T14:06:00Z","sig":"lena-k1","type":"item.confirm","payload":{"item":"lena#1","assignee":"ivy"}},
          {"author":"lena","seq":4,"ts":"2026-11-02T19:29:00Z","sig":"lena-k1","type":"item.create","payload":{"kind":"task","title":"Audit logs"}},
          {"author":"lena","seq":5,"ts":"2026-11-02T19:46:00Z","sig":"lena-k1","type":"item.assign","payload":{"item":"lena#4","mode":"chosen","levels":2,"people":["raj"],"completion":"assignee"}}
        ],
        "raj": [
          {"author":"raj","seq":1,"ts":"2026-11-02T11:50:00Z","sig":"raj-k1","type":"item.create","payload":{"kind":"task","title":"Query path rewrite","parent":"ada#1"}},
          {"author":"raj","seq":2,"ts":"2026-11-02T12:07:00Z","sig":"raj-k1","type":"item.assign","payload":{"item":"raj#1","mode":"chosen","levels":1,"people":["sven","tia"],"completion":"assignee"}},
          {"author":"raj","seq":3,"ts":"2026-11-02T21:28:00Z","sig":"raj-k1","type":"item.create","via":"agent","auto":true,"payload":{"kind":"note","title":"auto note"}}
        ],
        "ivy": [
          {"author":"ivy","seq":1,"ts":"2026-11-02T12:24:00Z","sig":"ivy-k1","type":"progress.report","via":"agent","auto":true,"payload":{"item":"lena#1","pct":40,"note":"regex tokenizer drafted"}},
          {"author":"ivy","seq":2,"ts":"2026-11-02T13:32:00Z","sig":"ivy-k1","type":"progress.report","via":"agent","auto":true,"payload":{"item":"lena#1","pct":100,"note":"tests green"}},
          {"author":"ivy","seq":3,"ts":"2026-11-02T13:49:00Z","sig":"ivy-k1","type":"item.close","via":"agent","auto":true,"payload":{"item":"lena#1"}},
          {"author":"ivy","seq":4,"ts":"2026-11-02T18:38:00Z","sig":"ivy-k1","type":"progress.report","via":"agent","auto":true,"payload":{"item":"ada#5","pct":30}}
        ],
        "jon": [
          {"author":"jon","seq":1,"ts":"2026-11-02T12:41:00Z","sig":"jon-k1","type":"progress.report","payload":{"item":"lena#1","pct":30,"note":"stemmer spike"}},
          {"author":"ada","seq":2,"ts":"2026-11-02T21:11:00Z","sig":"ada-k1","type":"progress.report","payload":{"item":"lena#1","pct":99,"note":"forged author"}}
        ],
        "omar": [
          {"author":"omar","seq":1,"ts":"2026-11-02T12:58:00Z","sig":"omar-k1","type":"progress.report","via":"agent","auto":true,"payload":{"item":"maya#1","pct":60,"note":"shard writer done"}}
        ],
        "kai": [
          {"author":"kai","seq":1,"ts":"2026-11-02T13:15:00Z","type":"progress.report","payload":{"item":"maya#1","pct":10,"note":"not assigned to kai"}},
          {"author":"kai","seq":2,"ts":"2026-11-02T17:47:00Z","sig":"kai-k1","type":"item.close","payload":{"item":"ada#5"}}
        ],
        "sven": [
          {"author":"sven","seq":1,"ts":"2026-11-02T14:23:00Z","sig":"sven-k1","type":"progress.report","payload":{"item":"raj#1","pct":50,"note":"planner skeleton"}},
          {"author":"sven","seq":2,"ts":"2026-11-02T16:22:00Z","sig":"sven-k1","type":"progress.report","payload":{"item":"bo#1","pct":90,"note":"sven is not an assignee of this"}}
        ],
        "tia": [
          {"author":"tia","seq":1,"ts":"2026-11-02T14:40:00Z","sig":"tia-k1","type":"item.create","payload":{"kind":"task","title":"Cache layer","parent":"raj#1"}},
          {"author":"tia","seq":2,"ts":"2026-11-02T14:57:00Z","sig":"tia-k1","type":"item.assign","payload":{"item":"tia#1","mode":"chosen","levels":1,"people":["pia"],"completion":"assignee"}},
          {"author":"tia","seq":3,"ts":"2026-11-02T20:03:00Z","sig":"tia-k1","type":"roles.set_lead","payload":{"member":"tia","leads":["ada"]}}
        ],
        "pia": [
          {"author":"pia","seq":1,"ts":"2026-11-02T15:14:00Z","sig":"pia-k1","type":"progress.report","via":"agent","auto":true,"payload":{"item":"tia#1","pct":20,"note":"LRU sketch"}}
        ],
        "bo": [
          {"author":"bo","seq":1,"ts":"2026-11-02T15:31:00Z","sig":"bo-k1","type":"item.create","payload":{"kind":"task","title":"Rotate CI keys","project":"infra"}},
          {"author":"bo","seq":2,"ts":"2026-11-02T15:48:00Z","sig":"bo-k1","type":"item.assign","payload":{"item":"bo#1","mode":"chosen","levels":1,"people":["uma"],"completion":"assignee"}},
          {"author":"bo","seq":3,"ts":"2026-11-02T20:20:00Z","sig":"bo-k1","type":"roles.set_lead","payload":{"member":"ada","leads":["ivy"]}},
          {"author":"bo","seq":4,"ts":"2026-11-02T20:37:00Z","sig":"bo-k1","type":"roles.set_lead","payload":{"member":"sven","leads":["maya","raj"]}},
          {"author":"bo","seq":5,"ts":"2026-11-02T20:54:00Z","sig":"bo-k1","type":"roles.set_lead","payload":{"member":"uma","leads":["raj"]}}
        ],
        "uma": [
          {"author":"uma","seq":1,"ts":"2026-11-02T16:05:00Z","sig":"uma-k1","type":"progress.report","payload":{"item":"bo#1","pct":50,"note":"staging keys rotated"}},
          {"author":"uma","seq":2,"ts":"2026-11-02T16:39:00Z","sig":"uma-k1","type":"item.close","payload":{"item":"bo#1"}},
          {"author":"uma","seq":3,"ts":"2026-11-02T16:56:00Z","sig":"uma-k1","type":"progress.report","payload":{"item":"bo#1","pct":20,"note":"late report after close"}},
          {"author":"uma","seq":6,"ts":"2026-11-02T22:02:00Z","sig":"uma-k1","type":"item.create","payload":{"kind":"todo","title":"Seq gap demo"}}
        ],
        "noa": [
          {"author":"noa","seq":1,"ts":"2026-11-02T18:04:00Z","sig":"ivy-k1","type":"item.close","payload":{"item":"ada#5"}},
          {"author":"noa","seq":2,"ts":"2026-11-02T18:21:00Z","sig":"noa-k1","type":"progress.report","payload":{"item":"ada#5","pct":70,"note":"half way"}}
        ]
      }
    },
    {
      name: "hana-personal",
      label: "hana-personal (personal store)",
      defaultViewer: "hana",
      roles: {
        "admins": ["hana"],
        "members": [
          {"id":"hana","name":"Hana Seo","keys":["hana-k1"],"leads":[]}
        ]
      },
      logs: {
        "hana": [
          {"author":"hana","seq":1,"ts":"2026-11-02T09:17:00Z","sig":"hana-k1","type":"item.create","payload":{"kind":"task","title":"Write talk proposal","project":"conf-2027","body":"Outline, abstract, bio."}},
          {"author":"hana","seq":2,"ts":"2026-11-02T09:34:00Z","sig":"hana-k1","type":"item.create","payload":{"kind":"note","title":"Reading list","body":"Event sourcing, CRDT survey, git internals."}},
          {"author":"hana","seq":3,"ts":"2026-11-02T09:51:00Z","sig":"hana-k1","type":"item.create","payload":{"kind":"todo","title":"Renew passport"}},
          {"author":"hana","seq":4,"ts":"2026-11-02T10:08:00Z","sig":"hana-k1","type":"item.create","payload":{"kind":"todo","title":"Try the tracker mockup"}},
          {"author":"hana","seq":5,"ts":"2026-11-02T10:25:00Z","sig":"hana-k1","type":"progress.report","via":"agent","auto":true,"payload":{"item":"hana#1","pct":35,"note":"outline done"}},
          {"author":"hana","seq":6,"ts":"2026-11-02T10:42:00Z","sig":"hana-k1","type":"item.close","payload":{"item":"hana#3"}}
        ]
      }
    }
  ]
};
