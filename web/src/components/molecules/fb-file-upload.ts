import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import './../atoms/fb-button.js';

/** A file that passed the client-side guard, ready to hand to the caller. */
export interface AcceptedFile {
  file: File;
  name: string;
  size: number;
}

/**
 * `fb-file-upload` — drag-drop + click file picker for images/PDFs (DSC §7.9),
 * used by invoice extraction. Does the client-side type/size guard and emits a
 * composed `files` ({ accepted, rejected }); the actual upload/progress is owned
 * by the caller (it holds the API client + request_id). The drop zone is a real
 * labelled button so keyboard users can open the picker.
 */
@customElement('fb-file-upload')
export class FbFileUpload extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .zone {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: var(--fb-spacing-sm);
        width: 100%;
        padding: var(--fb-spacing-lg);
        color: var(--fb-text-secondary);
        background: var(--fb-surface-1);
        border: 1px dashed var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
        cursor: pointer;
        text-align: center;
      }
      .zone:hover,
      .zone:focus-visible {
        border-color: var(--fb-gable-green);
        color: var(--fb-text-primary);
      }
      :host([dragging]) .zone {
        border-color: var(--fb-gable-green);
        background: color-mix(in srgb, var(--fb-gable-green) 8%, transparent);
      }
      .hint {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      input[type='file'] {
        display: none;
      }
      .rejected {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-sm);
        color: var(--fb-safety-red);
        font-size: var(--fb-text-body-sm);
      }
    `,
  ];

  /** Accept attribute + guard list (MIME or extension). */
  @property({ type: String }) accept = 'image/*,application/pdf';
  @property({ type: Number, attribute: 'max-size-mb' }) maxSizeMb = 10;
  @property({ type: Boolean }) multiple = false;
  @property({ type: String }) label = 'Drop files here or click to browse';
  @property({ type: Boolean, reflect: true }) disabled = false;

  @state() private dragging = false;
  @state() private rejectedMsg = '';

  private guard(files: FileList): { accepted: AcceptedFile[]; rejected: string[] } {
    const accepted: AcceptedFile[] = [];
    const rejected: string[] = [];
    const maxBytes = this.maxSizeMb * 1024 * 1024;
    const patterns = this.accept.split(',').map((s) => s.trim());
    for (const file of Array.from(files)) {
      const typeOk = patterns.some((p) =>
        p.endsWith('/*') ? file.type.startsWith(p.slice(0, -1)) : file.type === p || p === '',
      );
      if (!typeOk) rejected.push(`${file.name}: unsupported type`);
      else if (file.size > maxBytes) rejected.push(`${file.name}: over ${this.maxSizeMb}MB`);
      else accepted.push({ file, name: file.name, size: file.size });
    }
    return { accepted, rejected };
  }

  private handle(files: FileList | null): void {
    if (!files || files.length === 0) return;
    const { accepted, rejected } = this.guard(files);
    this.rejectedMsg = rejected.join('; ');
    if (accepted.length) this.emit('files', { accepted, rejected });
  }

  private reflectDragging(): void {
    this.toggleAttribute('dragging', this.dragging);
  }

  private onDrop(e: DragEvent): void {
    e.preventDefault();
    this.dragging = false;
    this.reflectDragging();
    if (!this.disabled) this.handle(e.dataTransfer?.files ?? null);
  }

  private onDragOver(e: DragEvent): void {
    e.preventDefault();
    if (this.disabled) return;
    this.dragging = true;
    this.reflectDragging();
  }

  private onDragLeave(): void {
    this.dragging = false;
    this.reflectDragging();
  }

  private openPicker(): void {
    if (this.disabled) return;
    this.renderRoot.querySelector<HTMLInputElement>('input[type=file]')?.click();
  }

  private onPicked(e: Event): void {
    this.handle((e.target as HTMLInputElement).files);
  }

  override render(): TemplateResult {
    return html`
      <button
        class="zone"
        type="button"
        ?disabled=${this.disabled}
        @click=${this.openPicker}
        @drop=${this.onDrop}
        @dragover=${this.onDragOver}
        @dragleave=${this.onDragLeave}
      >
        <fb-icon name="upload" size="28"></fb-icon>
        <span>${this.label}</span>
        <span class="hint">${this.accept} · up to ${this.maxSizeMb}MB</span>
      </button>
      <input
        type="file"
        accept=${this.accept}
        ?multiple=${this.multiple}
        @change=${this.onPicked}
      />
      ${this.rejectedMsg
        ? html`<p class="rejected" role="alert">
            <fb-icon name="alert-circle" size="14"></fb-icon>${this.rejectedMsg}
          </p>`
        : nothing}
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-file-upload': FbFileUpload;
  }
}
