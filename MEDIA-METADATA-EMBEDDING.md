# Media Metadata Embedding Implementation

## Overview

WarpNet implements a media metadata embedding system for images that helps establish accountability and traceability for uploaded content. This system is designed to work in conjunction with content moderation to prevent the spread of harmful content on the decentralized social network.

## Current Implementation

### Metadata Embedding Process

When a user uploads an image to WarpNet, the system automatically embeds encrypted metadata into the image's EXIF (Exchangeable Image File Format) segment. This process occurs transparently during the upload operation.

#### What Metadata is Embedded

The following information is embedded into each uploaded image:

1. **Node Information** (`nodeMetaKey: "node"`)
   - Node ID and network details
   - Information about the node handling the upload

2. **User Information** (`userMetaKey: "user"`)
   - User ID of the content creator
   - User profile data associated with the upload

3. **MAC Address** (`macMetaKey: "MAC"`)
   - Hardware network interface identifier
   - Additional device fingerprinting data

### Encryption Mechanism

The metadata embedding uses a deliberate "security through obscurity" approach with intentionally weak encryption:

#### Encryption Algorithm
- **Algorithm**: AES-256-GCM (Galois/Counter Mode)
- **Key Derivation**: Argon2id (when used with password) or time-based weak key generation
- **Password**: Randomly generated weak password that is **immediately discarded** after encryption
- **Salt**: Public, hardcoded salt (`"cec27db4"`) embedded with the media file
- **Nonce**: Zero-filled nonce (intentionally weak)

#### Security Model Philosophy

The system is designed with the following principles:

1. **Not Designed for User Decryption**: Ordinary users cannot recover the embedded metadata
2. **Designed for Powerful Entity Decryption**: Only entities with massive computational resources (e.g., government data centers, law enforcement with supercomputing access) can brute-force decrypt the metadata
3. **Proof of Ownership**: The encrypted EXIF metadata acts as proof of ownership and responsibility without revealing sensitive data during normal operation
4. **Computational Difficulty**: Security relies entirely on computational difficulty, not on secrecy of the password

From the source code comment in `core/handler/media.go`:

```
The system embeds encrypted metadata (node and user information) into the EXIF segment of media files
during upload.
A weak password is randomly generated for each file, used for encryption via Argon2id + AES-256-GCM,
and immediately discarded.
The password is never stored or logged.
Decryption is only possible through brute-force attacks, requiring massive computational resources.
Ordinary users cannot recover the metadata; only powerful entities (e.g., government data centers) can.
EXIF metadata acts as proof of ownership and responsibility without revealing sensitive data.
Salt and nonce are public and embedded with the media file.
Security relies entirely on computational difficulty, not on secrecy of the password.
```

### Technical Implementation Details

#### Image Upload Flow

1. **Image Reception** (`StreamUploadImageHandler`)
   - Receives base64-encoded image data
   - Maximum file size: 50 MiB
   - Supported formats: PNG, JPG, JPEG, GIF

2. **Image Processing**
   - Decodes the uploaded image
   - Re-encodes to JPEG format (Quality: 100) for consistency
   - This normalization ensures all images have EXIF capability

3. **Metadata Preparation**
   - Retrieves node information from the system
   - Fetches user data for the content owner
   - Collects MAC address of the uploading device
   - Serializes metadata to JSON

4. **Encryption**
   - Calls `security.EncryptAES(metaBytes, nil)` with `nil` password
   - This triggers weak key generation based on current timestamp
   - Encrypted data is base64-encoded

5. **EXIF Embedding**
   - Uses `amendExifMetadata()` function
   - Embeds encrypted metadata into `ImageDescription` EXIF tag
   - Writes modified JPEG with embedded EXIF data

6. **Storage**
   - Base64-encodes final image
   - Stores with generated SHA-256 hash as key
   - Returns image key to user

#### File Format
- All images are stored as JPEG with embedded EXIF data
- EXIF `ImageDescription` tag contains base64-encoded encrypted metadata
- Image keys are SHA-256 hashes of the final base64-encoded image

## Content Moderation Integration

### Moderation Policy

WarpNet implements LLM-based content moderation to prevent the spread of harmful content. The moderation system checks content against the following prohibited categories:

1. **CSAM and Minor Protection**
   - Child Sexual Abuse Material (CSAM)
   - Sexual content involving minors (including deepfakes or AI-generated)

2. **Non-Consensual Sexual Content**
   - Non-consensual sex
   - Pornography with coercion or abuse

3. **Violence and Gore**
   - Gore, extreme violence
   - Snuff content
   - Images of dead bodies

4. **Illegal Activities**
   - Weapon creation or sales
   - Drug creation or sales

5. **Self-Harm Content**
   - Self-harm promotion
   - Suicide encouragement
   - Eating disorder promotion

6. **Hate Speech and Discrimination**
   - Sexism against women only
   - Racism
   - Casteism
   - Xenophobia
   - General hate speech

7. **Extremism**
   - Religious extremism
   - Terrorism incitement

8. **Spam**
   - Mass unsolicited promotions
   - Spam content

### Moderation Engine

The moderation system uses a local LLM (Large Language Model) for content analysis:

- **Implementation**: Llama-based engine (`core/moderation/engine.go`)
- **Model**: Runs locally on the node
- **Analysis**: Evaluates text content against moderation policy
- **Response**: Binary decision (allow/block) with optional reason

#### Moderation Prompt Template

The system uses a structured prompt that:
- Defines the role as a social network moderator
- Lists all prohibited topics explicitly
- Requires English-only responses
- Expects either "Yes" (violation) with a short reason or "No" (acceptable)

## How Metadata Embedding Prevents Harmful Content

The metadata embedding system contributes to content safety through several mechanisms:

### 1. **Attribution and Accountability**
- Every image uploaded to WarpNet carries encrypted evidence of its origin
- Node and user information creates a chain of responsibility
- MAC address provides additional device-level tracking

### 2. **Deterrence Effect**
- Users aware of metadata embedding may be deterred from uploading harmful content
- Knowledge that content can be traced back to its source acts as a preventive measure

### 3. **Investigation Support**
- When harmful content is reported, metadata provides investigation leads
- Law enforcement or authorized entities can request brute-force decryption
- Decrypted metadata reveals the original uploader and their node

### 4. **Distributed Accountability**
- In a decentralized network, metadata helps identify responsible parties
- Prevents anonymity abuse while maintaining privacy for legitimate users

### 5. **Forensic Evidence**
- Embedded metadata can serve as forensic evidence in legal proceedings
- Provides proof of upload time, source node, and user identity
- MAC address adds physical device linkage

### 6. **Content Origin Verification**
- Helps distinguish original uploads from redistributed content
- Enables tracking of content spread across the network
- Supports identification of primary sources for harmful content

## Limitations and Considerations

### Privacy Implications
- All uploaded images contain embedded encrypted user information
- While encrypted, metadata can be decrypted with sufficient resources
- Users should be aware that images contain traceable information

### Security Limitations
- **Weak Encryption by Design**: The encryption is intentionally weak
- **No Forward Secrecy**: Once decrypted, all similar encryptions may be vulnerable
- **Metadata Removal**: Technically sophisticated users could strip EXIF data
- **Not Foolproof**: Determined malicious actors may find ways to circumvent the system

### Moderation Limitations
- **LLM-Based**: Subject to the limitations and biases of AI models
- **Text-Only**: Current implementation primarily moderates text content, not image content itself
- **Local Processing**: Effectiveness depends on the quality of the local LLM model
- **Language Barrier**: Moderation prompt expects English responses

## Technical Components

### Key Files and Modules

1. **`core/handler/media.go`**
   - Main image upload handler
   - EXIF metadata amendment
   - Encryption integration

2. **`security/weak-aes.go`**
   - Weak AES encryption implementation
   - Intentionally weak key generation
   - AES-256-GCM encryption

3. **`database/media-repo.go`**
   - Media storage and retrieval
   - Image key generation (SHA-256)
   - TTL-based foreign image caching

4. **`core/moderation/engine.go`**
   - LLM-based moderation engine
   - Content policy enforcement

5. **`core/moderation/prompt.go`**
   - Moderation prompt template
   - Policy definitions

### Dependencies

- **EXIF Processing**: `github.com/dsoprea/go-exif/v3`
- **JPEG Processing**: `github.com/dsoprea/go-jpeg-image-structure/v2`
- **Encryption**: Standard Go `crypto/aes` and `crypto/cipher`
- **LLM**: Custom Llama bindings (`core/moderation/binding/go-llama.cpp`)

## Future Enhancements

Potential improvements to the system could include:

1. **Image Content Analysis**: Extend moderation to analyze actual image content (not just text)
2. **Stronger Metadata Protection**: Optional user-controlled encryption for privacy-sensitive scenarios
3. **Watermarking**: Visible or invisible watermarking in addition to EXIF metadata
4. **Blockchain Integration**: Immutable record of content uploads on a blockchain
5. **Distributed Moderation**: Community-based content review systems
6. **Enhanced Forensics**: Additional metadata like geolocation, camera info, etc.

## Conclusion

WarpNet's media metadata embedding system balances privacy with accountability. By embedding encrypted user and node information in uploaded images, the system creates a deterrent against harmful content while maintaining reasonable privacy for legitimate users. Combined with LLM-based content moderation, this approach helps prevent the spread of prohibited content including CSAM, violence, hate speech, and other harmful materials on the decentralized social network.

The intentionally weak encryption ensures that while casual users cannot access the metadata, authorized entities with sufficient resources can decrypt it when investigating serious crimes or policy violations.
