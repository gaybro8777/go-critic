package checkers

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/go-critic/go-critic/checkers/internal/astwalk"
	"github.com/go-critic/go-critic/framework/linter"
	"github.com/go-toolsmith/astcast"
	"github.com/go-toolsmith/astequal"
)

func init() {
	var info linter.CheckerInfo
	info.Name = "unlambda"
	info.Tags = []string{"style"}
	info.Summary = "Detects function literals that can be simplified"
	info.Before = `func(x int) int { return fn(x) }`
	info.After = `fn`

	collection.AddChecker(&info, func(ctx *linter.CheckerContext) linter.FileWalker {
		return astwalk.WalkerForExpr(&unlambdaChecker{ctx: ctx})
	})
}

type unlambdaChecker struct {
	astwalk.WalkHandler
	ctx *linter.CheckerContext
}

func (c *unlambdaChecker) VisitExpr(x ast.Expr) {
	fn, ok := x.(*ast.FuncLit)
	if !ok || len(fn.Body.List) != 1 {
		return
	}

	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return
	}

	result := astcast.ToCallExpr(ret.Results[0])
	callable := qualifiedName(result.Fun)
	if callable == "" {
		return // Skip tricky cases; only handle simple calls
	}
	if isBuiltin(callable) {
		return // See #762
	}
	if id, ok := result.Fun.(*ast.Ident); ok {
		obj := c.ctx.TypesInfo.ObjectOf(id)
		if _, ok := obj.(*types.Var); ok {
			return // See #888
		}
	}
	fnType := c.ctx.TypeOf(fn)
	resultType := c.ctx.TypeOf(result.Fun)
	if !types.Identical(fnType, resultType) {
		return
	}
	// Now check that all arguments match the parameters.
	n := 0
	for _, params := range fn.Type.Params.List {
		if _, ok := params.Type.(*ast.Ellipsis); ok {
			if result.Ellipsis == token.NoPos {
				return
			}
			n++
			continue
		}

		for _, id := range params.Names {
			if !astequal.Expr(id, result.Args[n]) {
				return
			}
			n++
		}
	}

	if len(result.Args) == n {
		c.warn(fn, callable)
	}
}

func (c *unlambdaChecker) warn(cause ast.Node, suggestion string) {
	c.ctx.Warn(cause, "replace `%s` with `%s`", cause, suggestion)
}
