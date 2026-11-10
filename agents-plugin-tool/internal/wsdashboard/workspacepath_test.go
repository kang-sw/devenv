package wsdashboard

import "testing"

// TestNormalizeWorkspacePath pins how a pasted path is read on each host:
// what every host strips and refuses, the drive and distribution forms WSL
// maps into its own tree, the drive forms native Windows keeps, and that a
// POSIX host takes only an absolute path and "~".
func TestNormalizeWorkspacePath(t *testing.T) {
	none := func(string) bool { return false }
	hasC := func(p string) bool { return p == "/c" }
	wsl := pathHost{Distro: "Ubuntu", Home: "/home/me", IsDir: none}
	win := pathHost{Windows: true, Home: `C:\Users\me`, IsDir: none}
	linux := pathHost{Home: "/home/me", IsDir: none}
	mac := pathHost{Home: "/Users/me", IsDir: none}

	cases := []struct {
		name    string
		host    pathHost
		in      string
		want    string
		refusal pathRefusal
	}{
		// Every host.
		{"empty", linux, "   ", "", pathEmpty},
		{"only quotes", linux, `""`, "", pathEmpty},
		{"double quotes and trailing slash", linux, `"/home/me/work/"`, "/home/me/work", ""},
		{"single quotes and space", linux, `  '/home/me/work'  `, "/home/me/work", ""},
		{"dot elements are cleaned", linux, "/home/me/./a/../work//api/", "/home/me/work/api", ""},
		{"root", linux, "/", "/", ""},
		{"tilde", linux, "~", "/home/me", ""},
		{"tilde path", mac, "~/work/api/", "/Users/me/work/api", ""},
		{"tilde with no home", pathHost{IsDir: none}, "~/x", "", pathRelative},
		{"another user's tilde", linux, "~other/x", "", pathRelative},
		{"relative", linux, "work/api", "", pathRelative},
		{"dot relative", linux, "./work", "", pathRelative},
		{"network share", linux, `\\fileserver\share\x`, "", pathShare},
		{"a line break", linux, "/home/me\n/x", "", pathRelative},

		// Linux and macOS: only an absolute path and "~".
		{"linux refuses a drive path", linux, `C:\work\api`, "", pathRelative},
		{"mac refuses a drive path", mac, "C:/work/api", "", pathRelative},
		{"linux keeps /c as a folder", linux, "/c/work", "/c/work", ""},
		{"linux keeps /mnt/C as typed", linux, "/mnt/C/work", "/mnt/C/work", ""},
		{"linux refuses a wsl share", linux, `\\wsl$\Ubuntu\home\me`, "", pathShare},
		{"linux reads a doubled slash as a path", linux, "//home/me", "/home/me", ""},
		{"linux leaves a backslash tilde relative", linux, `~\work`, "", pathRelative},

		// WSL.
		{"wsl backslash drive", wsl, `C:\Users\me\work\api`, "/mnt/c/Users/me/work/api", ""},
		{"wsl quoted backslash drive", wsl, `"C:\Users\me\work\api\"`, "/mnt/c/Users/me/work/api", ""},
		{"wsl slash drive", wsl, "D:/work/api/", "/mnt/d/work/api", ""},
		{"wsl bare drive", wsl, "C:", "/mnt/c", ""},
		{"wsl drive-relative", wsl, "C:work", "", pathRelative},
		{"wsl msys drive", wsl, "/c/Users/me", "/mnt/c/Users/me", ""},
		{"wsl msys drive root", wsl, "/c", "/mnt/c", ""},
		{"wsl msys drive when /c is a folder", pathHost{Distro: "Ubuntu", IsDir: hasC}, "/c/Users/me", "/c/Users/me", ""},
		{"wsl mnt path", wsl, "/mnt/c/Users/me/", "/mnt/c/Users/me", ""},
		{"wsl mnt path, upper drive", wsl, "/mnt/C/Users/me", "/mnt/c/Users/me", ""},
		{"wsl linux path", wsl, "/home/me/devenv", "/home/me/devenv", ""},
		{"wsl two-letter top folder is no drive", wsl, "/cc/x", "/cc/x", ""},
		{"wsl$ share of this distribution", wsl, `\\wsl$\Ubuntu\home\me\devenv`, "/home/me/devenv", ""},
		{"wsl.localhost share of this distribution", wsl, `\\wsl.localhost\Ubuntu\home\me\devenv\`, "/home/me/devenv", ""},
		{"wsl.localhost share, forward slashes", wsl, "//wsl.localhost/ubuntu/home/me", "/home/me", ""},
		{"wsl share root", wsl, `\\wsl$\Ubuntu`, "/", ""},
		{"wsl share of another distribution", wsl, `\\wsl$\Debian\home\me`, "", pathDistro},
		{"wsl share with no distribution", wsl, `\\wsl.localhost\`, "", pathDistro},
		{"wsl network share", wsl, `\\fileserver\share\x`, "", pathShare},
		{"wsl tilde with backslashes", wsl, `~\work\api`, "/home/me/work/api", ""},
		{"wsl relative", wsl, `work\api`, "", pathRelative},

		// Native Windows.
		{"windows drive path", win, `C:\Users\me\work\api\`, `C:\Users\me\work\api`, ""},
		{"windows lower drive, forward slashes", win, "c:/Users/me//work/./api", `C:\Users\me\work\api`, ""},
		{"windows quoted drive path", win, `"D:\work\api"`, `D:\work\api`, ""},
		{"windows bare drive", win, "D:", `D:\`, ""},
		{"windows drive-relative", win, "C:work", "", pathRelative},
		{"windows msys drive", win, "/c/Users/me", `C:\Users\me`, ""},
		{"windows msys drive when \\c is a folder", pathHost{Windows: true, IsDir: hasC}, "/c/Users/me", "", pathRelative},
		{"windows rooted path on no drive", win, `\Users\me`, "", pathRelative},
		{"windows mnt path is on no drive", win, "/mnt/c/Users/me", "", pathRelative},
		{"windows network share", win, `\\fileserver\share\x`, "", pathShare},
		{"windows forward-slash share", win, "//fileserver/share/x", "", pathShare},
		{"windows wsl share", win, `\\wsl$\Ubuntu\home\me`, "", pathShare},
		{"windows tilde", win, "~", `C:\Users\me`, ""},
		{"windows tilde path", win, `~\work/api`, `C:\Users\me\work\api`, ""},
		{"windows relative", win, `work\api`, "", pathRelative},
	}
	for _, c := range cases {
		got, refusal := normalizeWorkspacePath(c.in, c.host)
		if got != c.want || refusal != c.refusal {
			t.Errorf("%s: normalizeWorkspacePath(%q) = %q, %q; want %q, %q", c.name, c.in, got, refusal, c.want, c.refusal)
		}
	}
}
