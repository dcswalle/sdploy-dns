# Automated PR Review Process

This repository uses automated code reviews to ensure code quality, security, and consistency. Every pull request is automatically reviewed using multiple tools and AI-powered analysis.

## Overview

When you open a pull request, the following automated checks are triggered:

1. **AI Code Review** - Claude AI analyzes your changes for security issues, bugs, and REST API best practices
2. **Linting** - Go code is linted using golangci-lint with comprehensive rules
3. **SLSA Build** - Supply chain security provenance is generated for the build

## AI Code Review

The AI review runs three parallel analyses on every PR:

### 🔒 Security Review
- Authentication and authorization issues
- SQL injection, command injection vulnerabilities
- Secrets in code
- Privilege boundary violations
- Unsafe default configurations

### 🐛 Bug Review
- Correctness issues
- Edge cases not handled
- Race conditions and concurrency bugs
- Error handling problems

### 🌐 REST API Practices Review
- HTTP semantics and status codes
- Request/response validation
- API consistency
- Idempotency
- Pagination and filtering conventions
- API versioning hygiene

### Review Results

The AI review results are posted as comments on your PR. Each review includes:

- **Severity levels**: critical, high, medium, low
- **Findings**: Concrete issues with file paths and line references
- **Suggested fixes**: How to address each issue
- **Quick wins**: Easy improvements
- **Residual risks**: Remaining concerns

### Addressing Review Comments

1. Review all findings posted by the AI reviewer
2. Address all **critical** and **high** severity issues
3. Consider addressing **medium** and **low** severity issues
4. Update the PR checklist to confirm you've reviewed the comments
5. Push your fixes - the review will run again automatically

## Linting

The repository uses `golangci-lint` with the following enabled linters:

- **errcheck** - Unchecked errors
- **gosimple** - Code simplification
- **govet** - Go vet checks
- **ineffassign** - Ineffectual assignments
- **staticcheck** - Static analysis
- **unused** - Unused code
- **gofmt** - Code formatting
- **goimports** - Import formatting
- **misspell** - Spelling
- **gosec** - Security issues
- **revive** - General linting
- **stylecheck** - Style consistency

### Running Linting Locally

Before pushing your PR, run the linter locally:

```bash
golangci-lint run
```

Or with automatic fixes:

```bash
golangci-lint run --fix
```

## Code Owners

This repository uses a CODEOWNERS file to automatically request reviews from the appropriate team members. When you open a PR:

- The file owners are automatically added as reviewers
- You must wait for approval from at least one code owner
- Security-sensitive files require review from repository owners

## PR Template

When creating a PR, use the provided template to:

- Describe your changes clearly
- Mark the type of change
- Link related issues
- Confirm testing was performed
- Address security considerations
- Complete the checklist

## Required Checks

Before your PR can be merged, the following checks must pass:

1. ✅ All AI reviews completed
2. ✅ Linting passes with no errors
3. ✅ SLSA build succeeds
4. ✅ At least one code owner approves
5. ✅ All critical/high severity issues addressed

## Configuration Files

- `.github/workflows/pr-review.yml` - AI review workflow
- `.github/workflows/lint.yml` - Linting workflow
- `.github/CODEOWNERS` - Code ownership definitions
- `.github/PULL_REQUEST_TEMPLATE.md` - PR template
- `.golangci.yml` - Linter configuration

## Secrets Required

For the AI review to work, the following GitHub secret must be configured:

- `ANTHROPIC_API_KEY` - API key for Claude AI

Repository administrators should configure this in: Settings → Secrets and variables → Actions

## Manual Review Script

You can also run the AI review script locally:

```bash
export ANTHROPIC_API_KEY="your-key-here"
./scripts/ci/claude_review.sh security output.md
./scripts/ci/claude_review.sh bugs output.md
./scripts/ci/claude_review.sh rest_api_practices output.md
```

## Troubleshooting

### AI Review Failed

If the AI review fails to post comments:

1. Check that `ANTHROPIC_API_KEY` is properly configured
2. Review the workflow logs in the Actions tab
3. Verify the script has access to changed files

### Linting Errors

If linting fails:

1. Run `golangci-lint run` locally to see all issues
2. Fix the issues or add exclusions if they're false positives
3. Push your fixes

### Build Failures

If the SLSA build fails:

1. Check that the code compiles: `go build -mod=vendor`
2. Review the workflow logs
3. Ensure all dependencies are properly vendored

## Best Practices

1. **Keep PRs small** - Smaller PRs are easier to review and have better AI analysis
2. **Address review comments promptly** - Don't ignore AI-generated findings
3. **Run linting locally** - Catch issues before pushing
4. **Test thoroughly** - The repository has no automated tests, so manual testing is critical
5. **Write clear descriptions** - Help reviewers understand your changes
6. **Review your own code first** - Self-review before requesting others

## Additional Resources

- [golangci-lint documentation](https://golangci-lint.run/)
- [GitHub Actions documentation](https://docs.github.com/en/actions)
- [CODEOWNERS documentation](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)
