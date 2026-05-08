import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { FormEvent, KeyboardEvent } from "react";
import { Mic, SendHorizontal, Upload } from "lucide-react";

type ReplyComposerProps = {
  value: string;
  disabled: boolean;
  pending: boolean;
  canReply: boolean;
  onChange: (value: string) => void;
  onSubmit: (event: FormEvent) => void;
  onMicClick?: () => void;
  onAttachFile?: (file: File) => void;
};

export default function ReplyComposer({
  value,
  disabled,
  pending,
  canReply,
  onChange,
  onSubmit,
  onMicClick,
  onAttachFile,
}: ReplyComposerProps) {
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const audioChunksRef = useRef<BlobPart[]>([]);
  const mediaStreamRef = useRef<MediaStream | null>(null);
  const [isRecording, setIsRecording] = useState(false);

  useLayoutEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "0px";
    const contentHeight = el.scrollHeight;
    const nextHeight = Math.max(44, Math.min(contentHeight, 180));
    el.style.height = `${nextHeight}px`;
    el.style.overflowY = contentHeight > nextHeight ? "auto" : "hidden";
  }, [value]);

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      if (!disabled && value.trim() && !pending) {
        const form = event.currentTarget.form;
        form?.requestSubmit();
      }
    }
  };

  useEffect(() => {
    return () => {
      if (mediaRecorderRef.current?.state === "recording") {
        mediaRecorderRef.current.stop();
      }
      mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
    };
  }, []);

  const handleMicToggle = async () => {
    if (disabled) return;
    if (isRecording) {
      mediaRecorderRef.current?.stop();
      setIsRecording(false);
      return;
    }

    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === "undefined") {
      onMicClick?.();
      return;
    }

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      mediaStreamRef.current = stream;
      audioChunksRef.current = [];

      const recorder = new MediaRecorder(stream);
      mediaRecorderRef.current = recorder;
      recorder.ondataavailable = (event) => {
        if (event.data && event.data.size > 0) {
          audioChunksRef.current.push(event.data);
        }
      };
      recorder.onstop = () => {
        setIsRecording(false);
        const mimeType = recorder.mimeType || "audio/webm";
        const blob = new Blob(audioChunksRef.current, { type: mimeType });
        if (blob.size > 0) {
          const extension = mimeType.includes("ogg") ? "ogg" : mimeType.includes("mp4") ? "m4a" : "webm";
          const file = new File([blob], `audio-${Date.now()}.${extension}`, { type: mimeType });
          onAttachFile?.(file);
        }
        audioChunksRef.current = [];
        mediaStreamRef.current?.getTracks().forEach((track) => track.stop());
        mediaStreamRef.current = null;
      };
      recorder.start();
      setIsRecording(true);
    } catch {
      onMicClick?.();
    }
  };

  const sendDisabled = disabled || !value.trim() || pending;

  return (
    <form className="atendimento-compose" onSubmit={onSubmit}>
      <div className={`atendimento-compose-card ${canReply ? "" : "blocked"}`}>
        <textarea
          ref={textareaRef}
          className="input atendimento-compose-input"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={canReply ? "Digite a resposta para o cliente" : "Assuma o atendimento para responder"}
          rows={1}
          disabled={disabled}
        />
        <div className="atendimento-compose-tools" role="group" aria-label="Acoes da mensagem">
          <button
            className={`atendimento-tool-button ${isRecording ? "recording" : ""}`}
            type="button"
            disabled={disabled}
            onClick={handleMicToggle}
            aria-label="Gravar audio"
            title="Gravar audio"
          >
            <Mic size={16} />
          </button>
          <button
            className="atendimento-tool-button"
            type="button"
            disabled={disabled}
            onClick={() => fileInputRef.current?.click()}
            aria-label="Enviar arquivo"
            title="Enviar arquivo"
          >
            <Upload size={16} />
          </button>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="atendimento-file-input"
            onChange={(event) => {
              const files = event.target.files;
              if (files && files.length > 0) {
                const firstFile = files.item(0);
                if (firstFile) {
                  onAttachFile?.(firstFile);
                }
              }
              event.currentTarget.value = "";
            }}
          />
          <button
            className="button atendimento-send-inline"
            type="submit"
            disabled={sendDisabled}
            aria-label="Enviar resposta"
            title="Enviar resposta"
          >
            {pending ? "..." : <SendHorizontal size={16} />}
          </button>
        </div>
      </div>
    </form>
  );
}
