# Shelly Enhancement Plan

## Overview
This plan outlines the implementation of features to enhance Shelly with:
1. Metasploit-like session management
2. Improved shell stability
3. Universal shell upgrades
4. Command isolation mechanism

## Current Architecture Review
- Main entry point: `src/main.rs`
- Shell handling: `src/shell.rs` (Netcat and Socat implementations)
- Helper functions: `src/helpers.rs`
- Configuration: `src/config.rs` and `shelly.json`
- HTTP serving: `src/serve.rs`

## Implementation Plan

### Phase 1: Session Management System
**Goal**: Implement Metasploit-like session listing, switching, and killing

#### Changes Needed:
1. Create session tracker module (`src/session.rs`)
   - Track active sessions with numeric IDs
   - Store session metadata (type, creation time, status)
   - Manage session lifecycle

2. Modify main loop to handle session commands
   - Add `sessions` command to list active sessions
   - Add `switch <id>` command to switch between sessions
   - Add `kill <id>` command to terminate sessions
   - Add `background` command (Ctrl+Z handling) to return to shelly prompt

3. Update Netcat and Socat implementations to:
   - Register new sessions when connections are established
   - Unregister sessions when they terminate
   - Support backgrounding/restoring sessions

### Phase 2: Command Isolation (Ctrl+Z Escape)
**Goal**: Prevent shelly commands from being sent to target host

#### Implementation:
1. Modify input handling in Netcat/Socat to detect Ctrl+Z (SIGTSTP)
2. When Ctrl+Z is detected:
   - Suspend current shell session
   - Return control to shelly prompt
   - Display session management prompt
3. Implement foreground/background session switching
4. Ensure proper terminal state management during transitions

### Phase 3: Universal Shell Upgrade Mechanism
**Goal**: Create consistent upgrade path across shell types

#### Implementation:
1. Design upgrade abstraction that works with:
   - Basic netcat shells
   - Socat shells
   - Other potential shell types
2. Common upgrade steps:
   - Spawn pty-aware process (Python, expect, etc.)
   - Transfer terminal control
   - Restore terminal state on exit
3. Make upgrade mechanism configurable per shell type

### Phase 4: Enhanced Stability & Auto-Listener Recreation
**Goal**: More stable shells with automatic recovery

#### Implementation:
1. Modify shell listeners to:
   - Automatically restart when a connection terminates
   - Continue listening for new connections
   - Maintain session history
2. Add connection tracking and statistics
3. Implement exponential backoff for failed listener starts
4. Add health checks for active sessions

### Phase 5: Configuration Updates
**Goal**: Support new features in configuration

#### Implementation:
1. Extend `shelly.json` to support:
   - Session timeout settings
   - Auto-reconnection configuration
   - Custom escape sequences (though we'll use Ctrl+Z as default)
   - Session history persistence

## Detailed Component Changes

### New Files:
- `src/session.rs`: Session tracking and management
- `src/upgrade.rs`: Universal shell upgrade mechanisms

### Modified Files:
- `src/main.rs`: Main loop integration with session management
- `src/shell.rs`: 
  - Netcat: Add session registration, Ctrl+Z handling, auto-restart
  - Socat: Add session registration, Ctrl+Z handling, auto-restart
- `src/helpers.rs`: Add terminal management utilities if needed
- `src/config.rs`: Extend for new session management options
- `src/serve.rs`: Minimal changes, if any

## Implementation Order

1. **Session Tracking Foundation**
   - Create session.rs with basic tracking
   - Modify main.rs to instantiate session manager
   - Add basic session listing command

2. **Command Isolation Mechanism**
   - Implement Ctrl+Z detection in shell handlers
   - Add background/foreground session switching
   - Test with simple netcat listener

3. **Session Lifecycle Management**
   - Add session registration/deregistration
   - Implement session killing
   - Add session switching capability

4. **Universal Upgrade System**
   - Create upgrade abstraction
   - Implement for netcat shells first
   - Extend to socat and other shell types

5. **Auto-Listener Recreation**
   - Modify shell run methods to restart on termination
   - Add connection attempt limits/backoff
   - Ensure clean resource cleanup

6. **Configuration Integration**
   - Add session management options to shelly.json
   - Make behaviors configurable where appropriate

## Testing Strategy
1. Unit tests for session tracking
2. Integration tests for session switching
3. Manual testing of Ctrl+Z escape mechanism
4. Testing upgrade paths with various shell types
5. Stress testing auto-reconnection capabilities

## Dependencies
- May need to add `nix` or `signal-hook` for proper signal handling
- Existing dependencies should suffice for most features

## Estimated Effort
- Phase 1 (Session Management): 2-3 days
- Phase 2 (Command Isolation): 1-2 days
- Phase 3 (Universal Upgrade): 2 days
- Phase 4 (Stability): 1-2 days
- Phase 5 (Configuration): 0.5 days

Total: ~6-8 days of development time