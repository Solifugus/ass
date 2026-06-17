// Package macro implements the SAS macro preprocessor. It runs before the
// lexer in the pipeline, handling %let, &var expansion, %macro/%mend
// definitions, and basic macro control flow.
package macro
