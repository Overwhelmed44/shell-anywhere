import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Check, ChevronDown } from "lucide-react";

type Option = { value: string; label: string; section?: string };

type Props = {
  label: string;
  value: string;
  options: Option[];
  onChange: (value: string) => void;
  icon?: ReactNode;
  optional?: boolean;
  disabled?: boolean;
  hint?: string;
  className?: string;
  placeholder?: string;
};

export default function Dropdown({
  label,
  value,
  options,
  onChange,
  icon,
  optional,
  disabled,
  hint,
  className = "",
  placeholder = "Choose an option",
}: Props) {
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const [placement, setPlacement] = useState({
    left: 0,
    top: 0,
    width: 0,
    maxHeight: 230,
  });
  const container = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const items = useRef<(HTMLButtonElement | null)[]>([]);
  const id = useId();
  const expanded = open && !disabled;
  const selected = options.find((option) => option.value === value);

  useLayoutEffect(() => {
    if (!expanded || !trigger.current || !menu.current) return;

    const rect = trigger.current.getBoundingClientRect();
    menu.current.style.width = `${rect.width}px`;
    const below = window.innerHeight - rect.bottom - 14;
    const above = rect.top - 14;
    const height = Math.min(menu.current.scrollHeight, 230);
    const upward = below < height && above > below;
    const maxHeight = Math.max(40, Math.min(230, upward ? above : below));

    setPlacement({
      left: rect.left,
      width: rect.width,
      top: upward
        ? rect.top - Math.min(height, maxHeight) - 6
        : rect.bottom + 6,
      maxHeight,
    });
  }, [expanded, options.length, hint]);

  useEffect(() => {
    if (!expanded) return;
    items.current[activeIndex]?.focus({ preventScroll: true });
    items.current[activeIndex]?.scrollIntoView({ block: "nearest" });
  }, [expanded, activeIndex]);

  useEffect(() => {
    if (!expanded) return;

    const outside = (event: PointerEvent) => {
      if (
        !container.current?.contains(event.target as Node) &&
        !menu.current?.contains(event.target as Node)
      )
        setOpen(false);
    };
    const close = () => setOpen(false);
    const scroll = (event: Event) => {
      if (!menu.current?.contains(event.target as Node)) close();
    };

    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", scroll, true);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", scroll, true);
    };
  }, [expanded]);

  const show = (index: number) => {
    setActiveIndex(Math.max(0, index));
    setOpen(true);
  };

  const select = (next: string) => {
    onChange(next);
    setOpen(false);
    trigger.current?.focus();
  };

  const navigate = (event: KeyboardEvent) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
      trigger.current?.focus();
    } else if (event.key === "Tab") {
      setOpen(false);
      trigger.current?.focus();
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const step = event.key === "ArrowDown" ? 1 : -1;
      setActiveIndex(
        (index) => (index + step + options.length) % options.length,
      );
    } else if (event.key === "Home" || event.key === "End") {
      event.preventDefault();
      setActiveIndex(event.key === "Home" ? 0 : options.length - 1);
    }
  };

  return (
    <div className={`field group-select-field ${className}`} ref={container}>
      <span id={`${id}-label`}>
        {label} {optional && <span className="optional">optional</span>}
      </span>
      <button
        ref={trigger}
        type="button"
        role="combobox"
        className={`group-select-trigger${expanded ? " open" : ""}`}
        aria-label={label}
        aria-describedby={`${id}-value`}
        aria-expanded={Boolean(expanded)}
        aria-haspopup="listbox"
        aria-controls={expanded ? `${id}-menu` : undefined}
        disabled={disabled || options.length === 0}
        onClick={() =>
          expanded
            ? setOpen(false)
            : show(options.findIndex((option) => option.value === value))
        }
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            show(
              event.key === "ArrowUp"
                ? options.length - 1
                : options.findIndex((option) => option.value === value),
            );
          }
        }}
      >
        {icon && <span className="group-select-icon">{icon}</span>}
        <span
          id={`${id}-value`}
          className={value ? "" : "group-select-placeholder"}
        >
          {selected?.label ?? placeholder}
        </span>
        <ChevronDown
          size={16}
          className="group-select-chevron"
          aria-hidden="true"
        />
      </button>
      {expanded &&
        createPortal(
          <div
            ref={menu}
            id={`${id}-menu`}
            className="group-select-menu dropdown-popup"
            role="listbox"
            aria-label={label}
            onKeyDown={navigate}
            style={placement}
          >
            {options.map((option, index) => (
              <div key={option.value} role="presentation">
                {option.section &&
                  option.section !== options[index - 1]?.section && (
                    <div className="dropdown-section">{option.section}</div>
                  )}
                <button
                  ref={(element) => {
                    items.current[index] = element;
                  }}
                  type="button"
                  role="option"
                  aria-selected={value === option.value}
                  tabIndex={-1}
                  className={`group-select-option${value === option.value ? " selected" : ""}`}
                  onClick={() => select(option.value)}
                >
                  <span>{option.label}</span>
                  {value === option.value && (
                    <Check size={16} aria-hidden="true" />
                  )}
                </button>
              </div>
            ))}
            {hint && <div className="group-select-hint">{hint}</div>}
          </div>,
          document.body,
        )}
    </div>
  );
}
