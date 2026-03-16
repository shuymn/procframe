package codegen

import (
	"testing"
)

func TestBuildTree_SingleServiceSingleMethod(t *testing.T) {
	t.Parallel()

	services := []serviceInfo{
		{
			GoName: "EchoService",
			Path:   []string{"echo"},
			Methods: []methodInfo{
				{
					GoName:       "Run",
					Path:         []string{"run"},
					CLI:          true,
					InputGoName:  "EchoRequest",
					OutputGoName: "EchoResponse",
					FullName:     "/test.v1.EchoService/Run",
				},
			},
		},
	}

	root := buildTree(services)

	echo, ok := root.Children["echo"]
	if !ok {
		t.Fatal("want echo group")
	}
	run, ok := echo.Children["run"]
	if !ok {
		t.Fatal("want run leaf under echo")
	}
	if run.Leaf == nil {
		t.Fatal("want run to be a leaf")
	}
	if run.Leaf.MethodGoName != "Run" {
		t.Fatalf("want MethodGoName=Run, got %q", run.Leaf.MethodGoName)
	}
}

func TestBuildTree_NestedPath(t *testing.T) {
	t.Parallel()

	services := []serviceInfo{
		{
			GoName:   "RepoPRService",
			Path:     []string{"repo", "pr"},
			BindInto: "pr",
			Summary:  "PR ops",
			Methods: []methodInfo{
				{
					GoName:       "List",
					Path:         []string{"list"},
					CLI:          true,
					Summary:      "List PRs",
					InputGoName:  "ListRequest",
					OutputGoName: "ListResponse",
					FullName:     "/test.v1.RepoPRService/List",
				},
			},
		},
	}

	root := buildTree(services)

	repo, ok := root.Children["repo"]
	if !ok {
		t.Fatal("want repo group")
	}
	pr, ok := repo.Children["pr"]
	if !ok {
		t.Fatal("want pr group under repo")
	}
	if pr.BindInto != "pr" {
		t.Fatalf("want BindInto=pr, got %q", pr.BindInto)
	}
	if pr.Summary != "PR ops" {
		t.Fatalf("want Summary=%q, got %q", "PR ops", pr.Summary)
	}
	list, ok := pr.Children["list"]
	if !ok {
		t.Fatal("want list leaf under pr")
	}
	if list.Leaf == nil {
		t.Fatal("want list to be a leaf")
	}
}

func TestBuildTree_DeadGroupPrune(t *testing.T) {
	t.Parallel()

	services := []serviceInfo{
		{
			GoName: "HiddenService",
			Path:   []string{"hidden"},
			Methods: []methodInfo{
				{
					GoName:       "DoStuff",
					Path:         []string{"do"},
					CLI:          false, // not exposed to CLI
					InputGoName:  "Req",
					OutputGoName: "Res",
					FullName:     "/test.v1.HiddenService/DoStuff",
				},
			},
		},
	}

	root := buildTree(services)

	if _, ok := root.Children["hidden"]; ok {
		t.Fatal("want dead group pruned")
	}
}

func TestBuildTree_MultipleMethods(t *testing.T) {
	t.Parallel()

	services := []serviceInfo{
		{
			GoName: "RepoService",
			Path:   []string{"repo"},
			Methods: []methodInfo{
				{
					GoName: "List", Path: []string{"list"},
					CLI: true, InputGoName: "ListReq", OutputGoName: "ListRes",
					FullName: "/test.v1.RepoService/List",
				},
				{
					GoName: "Get", Path: []string{"get"},
					CLI: true, InputGoName: "GetReq", OutputGoName: "GetRes",
					FullName: "/test.v1.RepoService/Get",
				},
				{
					GoName: "Delete", Path: []string{"delete"},
					CLI: false, InputGoName: "DelReq", OutputGoName: "DelRes",
					FullName: "/test.v1.RepoService/Delete",
				},
			},
		},
	}

	root := buildTree(services)

	repo := root.Children["repo"]
	if repo == nil {
		t.Fatal("want repo group")
	}
	if len(repo.Children) != 2 {
		t.Fatalf("want 2 children (list, get), got %d", len(repo.Children))
	}
	if repo.Children["list"] == nil || repo.Children["list"].Leaf == nil {
		t.Fatal("want list leaf")
	}
	if repo.Children["get"] == nil || repo.Children["get"].Leaf == nil {
		t.Fatal("want get leaf")
	}
	if _, ok := repo.Children["delete"]; ok {
		t.Fatal("want delete excluded (cli=false)")
	}
}

func TestBuildTree_StreamingMethodIncluded(t *testing.T) {
	t.Parallel()

	services := []serviceInfo{
		{
			GoName: "ChatService",
			Path:   []string{"chat"},
			Methods: []methodInfo{
				{
					GoName: "Stream", Path: []string{"stream"},
					CLI: true, Shape: shapeServerStream,
					InputGoName: "ChatReq", OutputGoName: "ChatChunk",
					FullName: "/test.v1.ChatService/Stream",
				},
			},
		},
	}

	root := buildTree(services)
	chat := root.Children["chat"]
	if chat == nil {
		t.Fatal("want chat group")
	}
	stream := chat.Children["stream"]
	if stream == nil || stream.Leaf == nil {
		t.Fatal("want stream leaf")
	}
	if stream.Leaf.Shape != shapeServerStream {
		t.Fatalf("want Shape=server_stream, got %q", stream.Leaf.Shape)
	}
}
