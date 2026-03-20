package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const maxMessages = 500

// MessageQueue represents a circular buffer of log messages.
type MessageQueue struct {
	messages [maxMessages]LogMessage
	head     int // index of the oldest message
	tail     int // index where the next message will be written
	count    int // number of messages currently in the buffer

	ErrorCount     int
	MessagesScroll int
	HideStatusPill bool
	nextMsgID      int
}

// NewMessageQueue creates a new MessageQueue.
func NewMessageQueue() *MessageQueue {
	return &MessageQueue{}
}

// AddMessage appends a new message and returns a command to clear it from the status bar after a delay.
func (mq *MessageQueue) AddMessage(level MsgLevel, text string) tea.Cmd {
	mq.nextMsgID++
	id := fmt.Sprintf("%d", mq.nextMsgID)
	msg := LogMessage{
		Timestamp: time.Now(),
		Level:     level,
		Text:      text,
		ID:        id,
	}

	wasAtBottom := mq.MessagesScroll >= mq.count-1-15
	if wasAtBottom {
		mq.MessagesScroll++
	}

	if mq.count == maxMessages {
		removed := mq.messages[mq.head]
		if removed.Level == LevelError {
			mq.ErrorCount--
		}

		mq.messages[mq.tail] = msg
		mq.head = (mq.head + 1) % maxMessages
		mq.tail = (mq.tail + 1) % maxMessages

		if mq.MessagesScroll > 0 {
			mq.MessagesScroll--
		}
	} else {
		mq.messages[mq.tail] = msg
		mq.tail = (mq.tail + 1) % maxMessages
		mq.count++
	}

	if level == LevelError {
		mq.ErrorCount++
	}

	mq.HideStatusPill = false
	return clearStatusCmd(id)
}

// Messages returns a slice of the current log messages in chronological order.
func (mq *MessageQueue) Messages() []LogMessage {
	if mq.count == 0 {
		return nil
	}
	res := make([]LogMessage, mq.count)
	if mq.head < mq.tail {
		copy(res, mq.messages[mq.head:mq.tail])
	} else {
		n := copy(res, mq.messages[mq.head:maxMessages])
		copy(res[n:], mq.messages[0:mq.tail])
	}
	return res
}

// ClearStatusPill hides the status pill if the ID matches the latest message.
func (mq *MessageQueue) ClearStatusPill(id string) {
	msgs := mq.Messages()
	if len(msgs) > 0 && msgs[len(msgs)-1].ID == id {
		mq.HideStatusPill = true
	}
}
