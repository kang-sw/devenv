package wsredact

import "testing"

// TestURLCredentials pins the URL rule the tracker's remote display shares.
func TestURLCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://x-access-token:ghp_secret@github.com/o/r.git": "https://***@github.com/o/r.git",
		"git@github.com:o/r.git":                               "git@github.com:o/r.git",
		"https://user:p@ss@host/x.git":                         "https://***@host/x.git",
		"https://host/r.git?private_token=glpat-x&ref=main":    "https://host/r.git?private_token=***&ref=***",
		"see https://host/docs?q=1 and ssh://h/r?x=1":          "see https://host/docs?q=*** and ssh://h/r?x=1",
	} {
		if got := URLCredentials(in); got != want {
			t.Errorf("URLCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSecretsMasksEachRule pins every rule with a positive case.
func TestSecretsMasksEachRule(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"url userinfo", "git clone https://u:hunter2@host/r.git", "git clone https://***@host/r.git"},
		{"url query", "curl https://api/x?access_token=abc&v=1", "curl https://api/x?access_token=***&v=***"},
		{"aws key id", "AWS_ACCESS_KEY_ID AKIAIOSFODNN7EXAMPLE set", "AWS_ACCESS_KEY_ID *** set"},
		{"aws session key id", "id ASIAY34FZKBOKMUTVV7A", "id ***"},
		{"github classic", "echo ghp_abcdefghijklmnopqrstuvwxyz0123456789", "echo ***"},
		{"github oauth", "gho_16C7e42F292c6912E7710c838347Ae178B4a", "***"},
		{"github user-to-server", "ghu_16C7e42F292c6912E7710c838347Ae178B4a", "***"},
		{"github server", "x ghs_16C7e42F292c6912E7710c838347Ae178B4a y", "x *** y"},
		{"github refresh", "ghr_16C7e42F292c6912E7710c838347Ae178B4a", "***"},
		{"github fine-grained", "github_pat_11ABCDEFG0123456789_abcdefghijklmnop", "***"},
		{"github cut by the preview cap", "command: echo ghp_abcdefghij…", "command: echo ***…"},
		{"gitlab", "glpat-xxxxxxxxxxxxxxxxxxxx", "***"},
		{"slack", "xoxb-1234567890-abcdefghij", "***"},
		{"slack user", "xoxp-1234567890-abcdefghij", "***"},
		{"openai", "sk-abcdefghijklmnopqrstuvwxyz012345", "***"},
		{"openai project", "OPENAI sk-proj-abcdefghijklmnopqrstuvwxyz_01", "OPENAI ***"},
		{"anthropic", "sk-ant-api03-abcdefghijklmnop", "***"},
		{"google", "key AIzaSyA-1234567890abcdefghijklmnopqrs", "key ***"},
		{"jwt", "jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U done", "jwt *** done"},
		{"jwt cut", "t=eyJhbGciOiJIUzI1NiJ9.eyJzdW", "t=***"},
		{"pem block", "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY----- tail", "-----BEGIN RSA PRIVATE KEY-----***-----END RSA PRIVATE KEY----- tail"},
		{"pem cut", "content: -----BEGIN PRIVATE KEY----- MIIEvQIBADANBgkqhkiG9w0BAQEFAASC…", "content: -----BEGIN PRIVATE KEY-----***"},
		{"pem openssh", "-----BEGIN OPENSSH PRIVATE KEY-----b3BlbnNzaC1rZXk=-----END OPENSSH PRIVATE KEY-----", "-----BEGIN OPENSSH PRIVATE KEY-----***-----END OPENSSH PRIVATE KEY-----"},
		{"authorization bearer", `curl -H "Authorization: Bearer abc.def" x`, `curl -H "Authorization: Bearer ***" x`},
		{"authorization basic", "authorization=basic dXNlcjpwYXNz", "authorization=basic ***"},
		{"authorization token", "Authorization: token abc123", "Authorization: token ***"},
		{"key=value", "PASSWORD=hunter2 ./run", "PASSWORD=*** ./run"},
		{"prefixed key=value", "export DB_PASSWORD=hunter2", "export DB_PASSWORD=***"},
		{"env token", "GITHUB_TOKEN=abc make", "GITHUB_TOKEN=*** make"},
		{"key: value", "secret: s3cr3t, path: /a", "secret: ***, path: /a"},
		{"json pair", `{"api_key": "abc def", "n": 1}`, `{"api_key": "***", "n": 1}`},
		{"single quoted", "passwd='p w'", "passwd='***'"},
		{"apikey", "apikey=abc", "apikey=***"},
		{"api-key", "api-key: abc", "api-key: ***"},
		{"access_key", "aws_secret_access_key=abc", "aws_secret_access_key=***"},
		{"private_key", "private_key: abc", "private_key: ***"},
		{"client_secret", "client_secret=abc&x=1", "client_secret=***&x=1"},
		{"auth", "auth: abc", "auth: ***"},
		{"pwd", "pwd=abc", "pwd=***"},
		{"case-insensitive", "Token: abc", "Token: ***"},
		{"flag value", "mysql --password hunter2 -u root", "mysql --password *** -u root"},
		{"flag=value", "gh auth login --with-token=abc", "gh auth login --with-token=***"},
		{"flag quoted", `tool --api-key "a b"`, `tool --api-key "***"`},
	} {
		if got := Secrets(c.in); got != c.want {
			t.Errorf("%s: Secrets(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestSecretsLeavesOrdinaryTextAlone pins the negatives: text that only
// resembles a rule survives unchanged.
func TestSecretsLeavesOrdinaryTextAlone(t *testing.T) {
	for _, in := range []string{
		"file_path: /home/user/project/internal/wslive/state.go",
		"command: git show a7b53d6b2c1e4f5a6b7c8d9e0f1a2b3c4d5e6f7a",
		"id 123e4567-e89b-12d3-a456-426614174000",
		"the tokenizer splits tokens: 1000, max_tokens: 4096",
		"pattern: authorization flow, description: rotate the api key",
		"Bearer of bad news",
		"secret garden",
		"command: cd $(pwd) && ls",
		"git@github.com:o/r.git",
		"risk-assessment-for-the-quarterly-plan",
		"AKIA alone",
		"ghp_ short",
		"eyJ",
		"oauthlib: installed",
		"--token --verbose",
		"-----BEGIN PUBLIC KEY----- MIIBIjANBgkq",
		"https://github.com/o/r/pull/12",
	} {
		if got := Secrets(in); got != in {
			t.Errorf("Secrets(%q) = %q, want it unchanged", in, got)
		}
	}
}
