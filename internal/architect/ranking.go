package architect

// PlanRanking records how a plan was chosen when several architect models proposed plans. APW
// fills it after parsing; a model-supplied value is discarded.
type PlanRanking struct {
	// Chosen is the index in Proposals of the plan that was executed.
	Chosen    int               `json:"chosen"`
	Proposals []ProposalSummary `json:"proposals"`
}

// ProposalSummary describes one proposed plan and its score.
type ProposalSummary struct {
	Model     string   `json:"model"`
	PlanID    string   `json:"plan_id,omitempty"`
	Objective string   `json:"objective,omitempty"`
	Steps     int      `json:"steps,omitempty"`
	Files     []string `json:"files,omitempty"`
	// Score is the combined 0-100 score; Static is APW's rule-based score, Judge the judge model's
	// 0-10 rating (nil when no judge ran).
	Score   int      `json:"score"`
	Static  int      `json:"static"`
	Judge   *int     `json:"judge,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
	Error   string   `json:"error,omitempty"`
}
