import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

type OtpInputProps = {
  value: string;
  onChange: (value: string) => void;
  onComplete?: (value: string) => void;
  length?: number;
  disabled?: boolean;
  autoFocus?: boolean;
  className?: string;
};

export function OtpInput({
  value,
  onChange,
  onComplete,
  length = 6,
  disabled = false,
  autoFocus = false,
  className,
}: OtpInputProps) {
  const { t } = useTranslation("common");
  const inputsRef = useRef<Array<HTMLInputElement | null>>([]);
  const [focusedIndex, setFocusedIndex] = useState(autoFocus ? 0 : -1);
  const digits = value.padEnd(length, " ").slice(0, length).split("");

  useEffect(() => {
    if (autoFocus) {
      inputsRef.current[0]?.focus();
    }
  }, [autoFocus]);

  useEffect(() => {
    if (value.length === length && !value.includes(" ")) {
      onComplete?.(value);
    }
  }, [value, length, onComplete]);

  function updateDigit(index: number, char: string) {
    const next = digits.map((d, i) => (i === index ? char : d === " " ? "" : d)).join("").slice(0, length);
    onChange(next);
    if (char && index < length - 1) {
      inputsRef.current[index + 1]?.focus();
    }
  }

  function handleChange(index: number, raw: string) {
    const digit = raw.replace(/\D/g, "").slice(-1);
    updateDigit(index, digit);
  }

  function handleKeyDown(index: number, e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Backspace") {
      e.preventDefault();
      if (digits[index] !== " " && digits[index] !== "") {
        updateDigit(index, "");
      } else if (index > 0) {
        inputsRef.current[index - 1]?.focus();
        updateDigit(index - 1, "");
      }
    } else if (e.key === "ArrowLeft" && index > 0) {
      inputsRef.current[index - 1]?.focus();
    } else if (e.key === "ArrowRight" && index < length - 1) {
      inputsRef.current[index + 1]?.focus();
    }
  }

  function handlePaste(e: React.ClipboardEvent) {
    e.preventDefault();
    const pasted = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, length);
    if (!pasted) return;
    onChange(pasted);
    const focusIndex = Math.min(pasted.length, length - 1);
    inputsRef.current[focusIndex]?.focus();
  }

  return (
    <div className={cn("flex justify-center gap-2", className)}>
      {digits.map((digit, index) => (
        <input
          key={index}
          ref={(el) => {
            inputsRef.current[index] = el;
          }}
          type="text"
          inputMode="numeric"
          autoComplete={index === 0 ? "one-time-code" : "off"}
          maxLength={1}
          value={digit === " " ? "" : digit}
          disabled={disabled}
          aria-label={t("otp.digitAria", { index: index + 1, length })}
          className={cn(
            "h-12 w-10 rounded-md border bg-background text-center text-lg font-semibold tabular-nums",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            focusedIndex === index && "ring-2 ring-ring",
            disabled && "opacity-50"
          )}
          onFocus={() => setFocusedIndex(index)}
          onBlur={() => setFocusedIndex(-1)}
          onChange={(e) => handleChange(index, e.target.value)}
          onKeyDown={(e) => handleKeyDown(index, e)}
          onPaste={handlePaste}
        />
      ))}
    </div>
  );
}

export function useTotpCountdown(periodSec = 30): number {
  const [remaining, setRemaining] = useState(() => periodSec - (Math.floor(Date.now() / 1000) % periodSec));

  useEffect(() => {
    const tick = () => setRemaining(periodSec - (Math.floor(Date.now() / 1000) % periodSec));
    tick();
    const id = window.setInterval(tick, 1000);
    return () => window.clearInterval(id);
  }, [periodSec]);

  return remaining;
}
