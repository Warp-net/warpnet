# Warpnet Moderation System

This document describes the current implementation of content moderation in Warpnet.

## Overview

Warpnet implements a decentralized content moderation system using dedicated moderator nodes that employ AI models to evaluate content based on a defined moderation policy. The system is designed to help maintain content quality and safety across the network without relying on centralized control.

## Architecture

### Components

The moderation system consists of several key components:

#### 1. Moderator Nodes

Moderator nodes are specialized nodes in the Warpnet network that run moderation engines. They:
- Continuously monitor network peers for content that requires moderation
- Retrieve unmoderated content from member nodes
- Process content through AI models
- Publish moderation results back to the network

#### 2. Moderation Engine

The moderation engine is built using llama.cpp bindings and provides:
- LLM-based content analysis
- Binary moderation decisions (OK or FAIL)
- Reason generation for rejected content
- Support for LLAMA2 model

Engine configuration:
- Context size: 512 tokens
- Output tokens: 64 tokens
- Temperature: 0.0 (deterministic)
- Top P: 0.9
- Memory mapping enabled
- Low VRAM mode supported

#### 3. Moderation Protocol

The isolation protocol handles communication between moderator nodes and member nodes:
- Sends moderation results to content owners
- Publishes results to followers via pubsub
- Updates tweet metadata with moderation information

## Moderation Workflow

### Tweet Moderation

1. Moderator nodes periodically scan connected peers every 10 seconds
2. For each non-moderator peer:
   - Retrieve node information
   - Fetch up to 20 tweets per request
   - Skip already moderated content
   - Process unmoderated tweets sequentially
3. For each unmoderated tweet:
   - Generate a prompt with the tweet text
   - Run inference through the LLM engine
   - Parse the model response (Yes/No with optional reason)
   - Create moderation result
4. Send moderation result back to:
   - The original content owner via stream
   - All followers via pubsub

### User Description Moderation

User profile moderation is partially implemented:
- Designed to moderate username and bio text
- Currently in development (lurking function placeholder)
- Will follow similar workflow to tweet moderation

## Moderation Policy

The current moderation policy prohibits the following content:

- CSAM or sexual content involving minors including deepfakes or AI-generated
- Non-consensual sex, porn with coercion or abuse
- Gore, violence, snuff, dead bodies
- Weapon or drug creation or sales
- Self-harm, suicide, eating disorders
- Sexism against women only, racism, casteism, xenophobia, hate speech
- Religious extremism, terrorism incitement
- Spam, mass unsolicited promos

The policy is embedded in the prompt template and enforced by the LLM model.

## Prompt Engineering

The moderation system uses a structured prompt template:

```
Instruction: You act as a social network moderator.
Does the following content violate moderation policy?

[Policy list...]

Respond in English only.

If yes, answer: 'Yes' and add short reason, max 14 words.
If no, answer: 'No'
No other answer types accepted.

Content:
"""[content]"""

Possible Violations:
[context]

Answer:
```

The system expects responses in one of two formats:
- "No" - Content is acceptable
- "Yes [reason]" - Content violates policy with a brief explanation

## Data Structures

### ModerationResultEvent

Represents a moderation decision:
- Type: Object type (tweet, user, reply, image)
- Result: Boolean (OK or FAIL)
- Reason: Optional string explaining rejection
- Model: Type of AI model used (LLAMA2)
- UserID: Content owner identifier
- ObjectID: Content identifier

### TweetModeration

Stored with each tweet:
- ModeratorID: ID of the moderator node
- Model: AI model used for moderation
- IsOk: Boolean result
- Reason: Optional rejection reason
- TimeAt: Timestamp of moderation

### ModerationObjectType

Supported content types:
- ModerationUserType: User profiles and descriptions
- ModerationTweetType: Tweet text content
- ModerationReplyType: Reply text content
- ModerationImageType: Image content (planned)

## Event Handling

### Member Nodes

Member nodes handle incoming moderation results:
1. Receive ModerationResultEvent via stream
2. Update tweet with moderation metadata
3. Delete from timeline if content fails moderation
4. Create notification for content owner if rejected

### Moderator Nodes

Moderator nodes publish results:
1. Send result directly to content owner
2. Broadcast to followers via pubsub
3. Cache moderation status to avoid duplicate processing

## Caching Strategy

The moderation system maintains a cache to prevent redundant processing:
- Tracks moderated content by peer ID, content type, and cursor
- Prevents re-processing of already moderated content
- Improves efficiency by avoiding duplicate AI inference

## Build Configuration

The moderation engine is conditionally compiled:
- Build tag: `llama`
- Requires llama.cpp bindings
- Must be explicitly enabled during build

## Limitations and Future Work

Current limitations:
- User description moderation not yet active
- Image content moderation not implemented
- Single model support (LLAMA2)
- No appeals or review process
- Moderation decisions are final

Potential improvements:
- Multi-model support for better accuracy
- Configurable moderation policies
- Reputation system for moderators
- User-controlled moderation preferences
- Image and video content analysis
- Appeals and review mechanism

## Security Considerations

The moderation system:
- Runs on dedicated nodes separate from user content
- Uses deterministic model settings for consistency
- Publishes all decisions transparently
- Allows users to see moderation metadata
- Cannot directly delete content from member nodes
- Relies on member nodes to honor moderation results

## Performance

Typical moderation performance:
- Processes one tweet at a time per peer
- 10-second intervals between peer scans
- Model inference time varies by hardware
- Logged for monitoring and optimization

## Configuration

Moderator nodes require:
- Model path configuration
- Thread count for inference
- Network configuration (testnet or mainnet)
- Sufficient hardware for LLM inference

## Network Protocol

Moderation uses standard Warpnet protocols:
- PUBLIC_GET_INFO: Retrieve node information
- PUBLIC_GET_TWEETS: Fetch tweets for moderation
- PUBLIC_POST_MODERATION_RESULT: Submit moderation results

All communication happens over libp2p streams with protocol multiplexing.
