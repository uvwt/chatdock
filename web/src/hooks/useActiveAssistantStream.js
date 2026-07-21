import { useCallback } from 'react';
import { appendInlineReasoningPart, appendInlineTextPart } from '../lib/toolEvents.js';
import { useStreamTextDisplay } from './useStreamTextDisplay.js';

export function useActiveAssistantStream(setMessages) {
  const updateAssistant = useCallback((patcher) => {
    setMessages(previous => previous.map((message, index) => (
      index === previous.length - 1 && message.role === 'assistant-stream' ? patcher(message) : message
    )));
  }, [setMessages]);

  const appendStreamParts = useCallback((parts) => {
    if (!parts.length) return;
    updateAssistant(message => parts.reduce((next, part) => (
      part.kind === 'reasoning'
        ? appendInlineReasoningPart(next, part.text)
        : appendInlineTextPart(next, part.text)
    ), message));
  }, [updateAssistant]);

  const streamText = useStreamTextDisplay(appendStreamParts);
  const appendAnswer = useCallback((text) => {
    if (text) updateAssistant(message => appendInlineTextPart(message, text));
  }, [updateAssistant]);
  const appendReasoning = useCallback((text) => {
    if (text) updateAssistant(message => appendInlineReasoningPart(message, text));
  }, [updateAssistant]);

  return { updateAssistant, appendAnswer, appendReasoning, ...streamText };
}
