//go:build llama
// +build llama

package moderation

import "fmt"

const moderationPromptTemplate = `
	Instruction: Does the following content violate moderation policy?
	Prohibited topics: 
		- CSAM or sexual content involving minors (including deepfakes or AI-generated).
		- Non-consensual sex, porn with coercion or abuse.
		- Gore, violence, snuff, dead bodies.
		- Weapon/drug creation or sales.
		- Self-harm, suicide, eating disorders.
		- Sexism, racism, casteism, xenophobia, hate speech.
		- Religious extremism, terrorism incitement.
		- Spam, mass unsolicited promos.

	Respond in English only. 

	If yes, answer: 'Yes' and provide reason (ten words maximum)
	If no, answer: No
	No other answer types accepted.
	
	Content:
	"""%s"""
	
	Answer:
`

func generatePrompt(content string) string {
	return fmt.Sprintf(moderationPromptTemplate, content)
}
