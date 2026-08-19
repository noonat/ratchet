// Package convention holds tests that enforce this repo's conventions on its own
// source, rather than trusting that they were read.
//
// The reason is measured, not theoretical. In the harness that preceded this repo, a
// trap was recorded in a comment on the very field that carried it, and the same
// defect shipped anyway a day later: an indented worked example taught a model to
// copy indentation onto a line that had none, and one model scored 0 of 50 as a
// result. A comment describing a defect does not prevent it. A test does.
//
// So a convention here is written in docs/conventions.md and, wherever it can be
// checked mechanically, enforced below.
//
// Each check reports every offender it finds in one assertion. Asserting per offender
// instead stops at the first, because gomega fails fatally, and the rest of the file
// goes unreported until the next run.
package convention

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// goFiles returns every Go file in the repo, keyed by path relative to the root, with
// the FileSet they were parsed in so a check can name the line it rejects. Walking
// the tree means a convention covers code added later without anyone remembering to
// list it.
func goFiles(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	g := NewWithT(t)
	root, err := filepath.Abs("../..")
	g.Expect(err).NotTo(HaveOccurred())

	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == ".git" || info.Name() == "journals") {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = f
		return nil
	})
	g.Expect(err).NotTo(HaveOccurred())
	// Every check below runs one subtest per file, so an empty map is a green run that
	// examined nothing. The root is a path relative to this package, and moving the
	// package would produce exactly that.
	g.Expect(out).NotTo(BeEmpty(), "found no Go files to check")
	return fset, out
}

// TestTableTestsUseSubtests enforces the rule that a table test runs each row in
// its own t.Run with its own gomega instance.
//
// Binding gomega to the parent t means a failure is attributed to the function
// rather than the row, the row's name never appears, and the first failure stops
// the rest of the table. Creating it inside the closure fixes all three, and
// costs one line.
func TestTableTestsUseSubtests(t *testing.T) {
	fset, files := goFiles(t)
	for name, file := range files {
		if !strings.HasSuffix(name, "_test.go") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tableTestViolations(fset, file)).
				To(BeEmpty(), "a table's rows each run in their own t.Run, with their own gomega created inside the closure, so a failure names the row")
		})
	}
}

// tableTestViolations describes every place a table's rows are not each run in their
// own subtest with their own gomega.
//
// Three shapes break the rule and only the first announces itself. A NewWithT in a
// loop body gives each row a gomega and no name. A table loop that never calls t.Run
// asserts through a gomega made above it, which is the common shape and the one that
// reads as correct. A t.Run whose closure borrows a gomega from outside is the same
// defect wearing a subtest: the failure is still fatal to the parent, so the table
// still stops at the first bad row.
func tableTestViolations(fset *token.FileSet, file *ast.File) []string {
	var bad []string
	seen := map[string]bool{}
	report := func(pos token.Pos, msg string) {
		// A loop nested in another loop is visited by both, so the same call is
		// reported twice without this.
		line := fmt.Sprintf("line %d: %s", fset.Position(pos).Line, msg)
		if !seen[line] {
			seen[line] = true
			bad = append(bad, line)
		}
	}

	tables := tableVars(file)
	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch loop := n.(type) {
		case *ast.RangeStmt:
			body = loop.Body
		case *ast.ForStmt:
			body = loop.Body
		default:
			return true
		}
		for _, call := range callsOutside(body, "NewWithT") {
			report(call.Pos(), "NewWithT in a loop body outside t.Run")
		}

		rng, isRange := n.(*ast.RangeStmt)
		if !isRange {
			return true
		}
		subject, isName := rng.X.(*ast.Ident)
		if !isName || !tables[subject.Name] {
			return true
		}
		closure := subtestClosure(body)
		if closure == nil {
			report(rng.Pos(), "the rows of table "+subject.Name+" do not run in t.Run")
			return true
		}
		if len(callsOutside(closure.Body, "NewWithT")) == 0 {
			report(closure.Pos(), "this t.Run closure asserts through a gomega made outside it")
		}
		return true
	})
	return bad
}

// tableVars names every variable in the file assigned a slice of anonymous structs,
// which is the shape docs/conventions.md prescribes for a table. Judging the shape
// rather than the name means a table called anything is still a table.
func tableVars(file *ast.File) map[string]bool {
	names := map[string]bool{}
	record := func(targets []ast.Expr, values []ast.Expr) {
		for i, value := range values {
			if i >= len(targets) {
				return
			}
			lit, isLit := value.(*ast.CompositeLit)
			if !isLit || !isStructSlice(lit.Type) {
				continue
			}
			if id, isName := targets[i].(*ast.Ident); isName {
				names[id.Name] = true
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.AssignStmt:
			record(d.Lhs, d.Rhs)
		case *ast.ValueSpec:
			targets := make([]ast.Expr, 0, len(d.Names))
			for _, name := range d.Names {
				targets = append(targets, name)
			}
			record(targets, d.Values)
		}
		return true
	})
	return names
}

// callsOutside finds calls to a named function inside a block, without descending
// into function literals: a call inside a closure belongs to that closure and not to
// the block that holds it.
func callsOutside(block *ast.BlockStmt, name string) []*ast.CallExpr {
	var found []*ast.CallExpr
	ast.Inspect(block, func(n ast.Node) bool {
		if _, isFunc := n.(*ast.FuncLit); isFunc {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, isName := call.Fun.(*ast.Ident); isName && id.Name == name {
			found = append(found, call)
		}
		return true
	})
	return found
}

// subtestClosure returns the function literal a block passes to t.Run, or nil when
// the block never calls it.
func subtestClosure(block *ast.BlockStmt) *ast.FuncLit {
	var found *ast.FuncLit
	ast.Inspect(block, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || sel.Sel.Name != "Run" || len(call.Args) == 0 {
			return true
		}
		if lit, isLit := call.Args[len(call.Args)-1].(*ast.FuncLit); isLit {
			found = lit
		}
		return true
	})
	return found
}

// TestExportedIdentifiersAreDocumented is the other convention worth enforcing
// rather than reviewing: every exported type, function, method and struct field
// carries a doc comment that starts with its name. A visual pass over the harness
// that preceded this repo missed twenty of them.
func TestExportedIdentifiersAreDocumented(t *testing.T) {
	_, files := goFiles(t)
	for name, file := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(undocumentedExports(file)).
				To(BeEmpty(), "exported identifier has no doc comment, or one that does not start with its name")
		})
	}
}

func undocumentedExports(file *ast.File) []string {
	var bad []string
	starts := func(doc *ast.CommentGroup, name string) bool {
		if doc == nil {
			return false
		}
		return strings.HasPrefix(strings.TrimSpace(doc.Text()), name)
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.IsExported() && !starts(d.Doc, d.Name.Name) {
				bad = append(bad, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				if !starts(ts.Doc, ts.Name.Name) && !starts(d.Doc, ts.Name.Name) {
					bad = append(bad, ts.Name.Name)
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, f := range st.Fields.List {
					for _, fn := range f.Names {
						if fn.IsExported() && !starts(f.Doc, fn.Name) {
							bad = append(bad, ts.Name.Name+"."+fn.Name)
						}
					}
				}
			}
		}
	}
	return bad
}

// TestFunctionBodiesAreMultiLine enforces that a declared function opens and closes
// its braces on different lines.
//
// A one-line body reads as a value rather than as code, so the next person adds a
// statement and reformats the whole thing, and the diff hides what changed. Function
// literals are exempt: a tiny transform passed as an argument is the one place the
// compact form is clearer.
func TestFunctionBodiesAreMultiLine(t *testing.T) {
	fset, files := goFiles(t)
	for name, file := range files {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(oneLineFuncs(fset, file)).
				To(BeEmpty(), "function body opens and closes on one line: put the statement on its own line")
		})
	}
}

// oneLineFuncs names every declared function whose braces share a line. It looks at
// FuncDecl only, so a function literal keeps the compact form.
func oneLineFuncs(fset *token.FileSet, file *ast.File) []string {
	var bad []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		open := fset.Position(fn.Body.Lbrace).Line
		close := fset.Position(fn.Body.Rbrace).Line
		if open == close {
			bad = append(bad, fn.Name.Name)
		}
	}
	return bad
}

// TestTablesAreNamedNotInlined enforces two things about a table test's data: the
// slice is a variable rather than an expression inside the `range`, and its struct
// literals name their fields.
//
// An inline table puts the data between `range` and the loop body, so reading the
// loop means reading past the whole table. Positional fields make a row unreadable
// the moment there are more than two of them, and adding a field silently reassigns
// every existing value.
//
// The two checks run in separate subtests. Together in one, the first to fail hides
// the second, which is how the positional-field check went unproven once already.
func TestTablesAreNamedNotInlined(t *testing.T) {
	_, files := goFiles(t)
	for name, file := range files {
		t.Run(name+"/inlined table", func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(inlinedTables(file)).
				To(BeEmpty(), "composite literal inlined into a range: assign it to a variable first, so the loop body reads without scrolling past the data")
		})
		t.Run(name+"/positional fields", func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(unkeyedStructRows(file)).
				To(BeEmpty(), "struct literal without field names: name every field, one per line, so adding a field cannot silently reassign the others")
		})
	}
}

// inlinedTables reports a range whose subject is a composite literal rather than a
// name.
func inlinedTables(file *ast.File) []string {
	var bad []string
	ast.Inspect(file, func(n ast.Node) bool {
		rng, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		if _, isLiteral := rng.X.(*ast.CompositeLit); isLiteral {
			bad = append(bad, "range over a literal")
		}
		return true
	})
	return bad
}

// unkeyedStructRows reports rows of a slice-of-struct literal that use positional
// fields. It only judges literals whose element type is spelled in the source, since
// without type information a bare `Foo{1, 2}` cannot be told from `[]int{1, 2}`.
func unkeyedStructRows(file *ast.File) []string {
	var bad []string
	ast.Inspect(file, func(n ast.Node) bool {
		outer, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if !isStructSlice(outer.Type) {
			return true
		}
		for _, row := range outer.Elts {
			inner, ok := row.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, field := range inner.Elts {
				if _, keyed := field.(*ast.KeyValueExpr); !keyed {
					bad = append(bad, "positional field")
				}
			}
		}
		return true
	})
	return bad
}

// isStructSlice reports whether a type expression is a slice of anonymous structs,
// which is the shape a table test uses.
//
// Only the anonymous form is judged. A slice of a named type needs type resolution
// to tell a struct from an int, and the AST alone cannot do it: an earlier version
// guessed from the type's name, which is not a check.
func isStructSlice(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	_, anonymous := arr.Elt.(*ast.StructType)
	return anonymous
}
