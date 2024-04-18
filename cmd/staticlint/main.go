package main

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/framepointer"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"
)

// exitCheckAnalyzer запрещает использовать прямой вызов os.Exit в функции main пакета main.
var exitCheckAnalyzer = &analysis.Analyzer{
	Name: "exitCheck",
	Doc:  "check for direct use of os.Exit in the main function of the main package",
	Run:  exitCheckAnalyzerRun,
}

func exitCheckAnalyzerRun(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		if file.Name.Name == "main" {
			ast.Inspect(file, func(node ast.Node) bool {
				if mainFunc, ok := node.(*ast.FuncDecl); ok && mainFunc.Name.Name == "main" {
					inspectMainFunc(pass, mainFunc)
				}
				return true
			})
		}
	}
	return nil, nil
}

func inspectMainFunc(pass *analysis.Pass, mainFunc *ast.FuncDecl) {
	ast.Inspect(mainFunc, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if isExitCall(call) {
				pass.Reportf(call.Pos(), "direct use of os.Exit in main function of main package is not allowed")
			}
		}
		return true
	})
}

func isExitCall(call *ast.CallExpr) bool {
	if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := fun.X.(*ast.Ident); ok && ident.Name == "os" && fun.Sel.Name == "Exit" {
			return true
		}
	}
	return false
}

// Запустите multichecker, указав путь к файлам, которые вы хотите проверить. Например:
// ./multichecker path/to/your/code/*.go
// После запуска multichecker вы увидите вывод в терминале, где будут указаны найденные проблемы и предупреждения.
// Каждый анализатор будет работать независимо, поэтому вы можете видеть одинаковые предупреждения несколько раз, если они соответствуют нескольким анализаторам.
func main() {
	// Добавляем стандартные анализаторы пакета golang.org/x/tools/go/analysis/passes
	checks := []*analysis.Analyzer{
		asmdecl.Analyzer,
		framepointer.Analyzer,
		exitCheckAnalyzer, // Добавляем собственный анализатор
	}

	// Добавляем все анализаторы SA класса staticcheck.io
	for _, v := range staticcheck.Analyzers {
		if v.Analyzer.Name[:2] == "SA" {
			checks = append(checks, v.Analyzer)
		}
	}

	// Добавляем не менее одного анализатора остальных классов staticcheck.io
	// В данном примере мы добавим анализатор ST1000 из класса S
	for _, v := range staticcheck.Analyzers {
		if v.Analyzer.Name == "ST1000" {
			checks = append(checks, v.Analyzer)
			break
		}
	}

	// Добавляем два или более публичных анализаторов на выбор
	// В данном примере мы добавим анализаторы S1000 и QF1001 из пакета stylecheck
	for _, v := range stylecheck.Analyzers {
		if v.Analyzer.Name == "S1000" || v.Analyzer.Name == "QF1001" {
			checks = append(checks, v.Analyzer)
		}
	}

	multichecker.Main(checks...)
}
