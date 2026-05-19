package api

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// Ensure the rl_env handlers are only compiled when sandboxManager is available.
// All handlers guard against nil sandboxManager.

// Problem is a single coding task from the dataset.
type Problem struct {
	ID         string   `json:"id"`
	Prompt     string   `json:"prompt"`
	TestCases  []string `json:"test_cases"`
	Difficulty string   `json:"difficulty"` // easy | medium | hard
}

// built-in mini dataset; real usage loads from JSON files or DB
var defaultProblems = []Problem{
	{
		ID:         "he-1",
		Prompt:     "Write a Python function `add(a: int, b: int) -> int` that returns the sum of two integers.",
		TestCases:  []string{"assert add(1, 2) == 3", "assert add(-1, 1) == 0", "assert add(0, 0) == 0"},
		Difficulty: "easy",
	},
	{
		ID:     "he-2",
		Prompt: "Write a Python function `is_palindrome(s: str) -> bool` that returns True if the string is a palindrome.",
		TestCases: []string{
			`assert is_palindrome("racecar") == True`,
			`assert is_palindrome("hello") == False`,
			`assert is_palindrome("") == True`,
		},
		Difficulty: "easy",
	},
	{
		ID:     "he-3",
		Prompt: "Write a Python function `fizzbuzz(n: int) -> list` that returns a list of strings 1..n where multiples of 3 are 'Fizz', multiples of 5 are 'Buzz', and multiples of both are 'FizzBuzz'.",
		TestCases: []string{
			`assert fizzbuzz(3) == ['1', '2', 'Fizz']`,
			`assert fizzbuzz(5) == ['1', '2', 'Fizz', '4', 'Buzz']`,
			`assert fizzbuzz(15)[-1] == 'FizzBuzz'`,
		},
		Difficulty: "easy",
	},
	{
		ID:     "he-4",
		Prompt: "Write a Python function `binary_search(arr: list, target: int) -> int` that returns the index of target in a sorted list, or -1 if not found.",
		TestCases: []string{
			`assert binary_search([1,3,5,7,9], 5) == 2`,
			`assert binary_search([1,3,5,7,9], 6) == -1`,
			`assert binary_search([], 1) == -1`,
		},
		Difficulty: "medium",
	},
	{
		ID:     "he-5",
		Prompt: "Write a Python function `merge_sort(arr: list) -> list` that returns a sorted copy of the list using merge sort.",
		TestCases: []string{
			`assert merge_sort([3,1,4,1,5]) == [1,1,3,4,5]`,
			`assert merge_sort([]) == []`,
			`assert merge_sort([1]) == [1]`,
		},
		Difficulty: "medium",
	},
}

// rlEnvResetHandler: POST /api/rl/env/reset
// Creates a sandbox and returns a problem for the rollout worker.
func (h *APIHandler) rlEnvResetHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RunID      string `json:"run_id"`
		Difficulty string `json:"difficulty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	// Sample a problem
	problems := defaultProblems
	if body.Difficulty != "" {
		var filtered []Problem
		for _, p := range problems {
			if p.Difficulty == body.Difficulty {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) > 0 {
			problems = filtered
		}
	}
	problem := problems[rand.Intn(len(problems))]

	if h.sandboxManager == nil {
		http.Error(w, "sandbox manager not initialized", http.StatusServiceUnavailable)
		return
	}

	// Create a sandbox (30 min TTL per episode)
	sb, err := h.sandboxManager.CreateSandbox("python3", 1800)
	if err != nil {
		h.logger.Errorf("rlEnvReset: create sandbox: %v", err)
		http.Error(w, "failed to create sandbox: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"sandbox_id": sb.ID,
		"problem_id": problem.ID,
		"prompt":     problem.Prompt,
		"test_cases": problem.TestCases,
	})
}

// rlEnvStepHandler: POST /api/rl/env/step
// Executes submitted code in the sandbox against test cases and returns a reward.
func (h *APIHandler) rlEnvStepHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SandboxID string   `json:"sandbox_id"`
		Code      string   `json:"code"`
		TestCases []string `json:"test_cases"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.SandboxID == "" || body.Code == "" {
		http.Error(w, "sandbox_id and code required", http.StatusBadRequest)
		return
	}
	if h.sandboxManager == nil {
		http.Error(w, "sandbox manager not initialized", http.StatusServiceUnavailable)
		return
	}

	passed, total, stdout, stderr, exitCode := runTestCases(h, body.SandboxID, body.Code, body.TestCases)

	reward := 0.0
	if total > 0 {
		reward = float64(passed) / float64(total)
	}
	// Length penalty: discourage bloated solutions
	lengthPenalty := 0.0
	if len(body.Code) > 500 {
		lengthPenalty = float64(len(body.Code)-500) * 0.0001
	}
	reward = max(0, reward-lengthPenalty)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"reward":       reward,
		"done":         true,
		"passed_tests": passed,
		"total_tests":  total,
		"stdout":       stdout,
		"stderr":       stderr,
		"exit_code":    exitCode,
	})
}

// rlEnvCloseHandler: POST /api/rl/env/close
// Destroys the sandbox after an episode.
func (h *APIHandler) rlEnvCloseHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SandboxID string `json:"sandbox_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.SandboxID == "" {
		http.Error(w, "sandbox_id required", http.StatusBadRequest)
		return
	}
	if err := h.sandboxManager.DestroySandbox(body.SandboxID); err != nil {
		h.logger.Warnf("rlEnvClose: %v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// rlEnvProblemsHandler: GET /api/rl/env/problems — list available problems
func (h *APIHandler) rlEnvProblemsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(defaultProblems)
}

// rlEnvProblemHandler: GET /api/rl/env/problems/{id} — get a specific problem
func (h *APIHandler) rlEnvProblemHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	for _, p := range defaultProblems {
		if p.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(p)
			return
		}
	}
	http.Error(w, "problem not found", http.StatusNotFound)
}

// runTestCases uploads the submission + test runner to the sandbox and executes it.
// Returns (passed, total, stdout, stderr, exitCode).
func runTestCases(h *APIHandler, sandboxID, code string, testCases []string) (int, int, string, string, int) {
	if len(testCases) == 0 {
		// No test cases — do a basic syntax check
		result, err := h.sandboxManager.ExecInSandbox(sandboxID, code, "python3", 10)
		if err != nil || result.ExitCode != 0 {
			stderr := ""
			if result != nil {
				stderr = result.Stderr
			}
			return 0, 1, "", stderr, 1
		}
		return 1, 1, result.Stdout, result.Stderr, result.ExitCode
	}

	passed := 0
	var lastStdout, lastStderr string
	lastExitCode := 0

	for _, tc := range testCases {
		testScript := fmt.Sprintf(`%s

# test case
try:
    %s
    print("__PASS__")
except AssertionError as e:
    print(f"__FAIL__: {e}")
except Exception as e:
    print(f"__ERROR__: {e}")
`, code, strings.ReplaceAll(tc, "\n", "\n    "))

		result, err := h.sandboxManager.ExecInSandbox(sandboxID, testScript, "python3", 10)
		if err != nil {
			lastStderr = err.Error()
			lastExitCode = 1
			continue
		}
		lastStdout = result.Stdout
		lastStderr = result.Stderr
		lastExitCode = result.ExitCode
		if strings.Contains(result.Stdout, "__PASS__") {
			passed++
		}
	}

	return passed, len(testCases), lastStdout, lastStderr, lastExitCode
}

// max returns the larger of two float64 values.
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// init seeds the random source for problem sampling
func init() {
	rand.Seed(time.Now().UnixNano())
}
