package main

func docsCheck(build bool) check {
	return check{Name: "docs", OK: true, Optional: true, Detail: "not built yet"}
}

func checkUpdate(a *app, data map[string]any) error { return nil }
