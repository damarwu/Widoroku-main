import type { ReactNode } from 'react';
import { X } from 'lucide-react';

export type ModalSize = 'small' | 'medium' | 'wide';

/**
 * Bingkai modal yang dipakai bersama oleh halaman Data warga dan Kumpulan Ahad Pon.
 */
export default function Modal({
  title,
  subtitle,
  onClose,
  children,
  footer,
  size,
}: {
  title: string;
  subtitle?: string;
  onClose: () => void;
  children: ReactNode;
  footer?: ReactNode;
  size?: ModalSize;
}) {
  return (
    <div className="modal-backdrop" onClick={onClose} role="presentation">
      <div
        className={`modal${size ? ` ${size}` : ''}`}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="modal-header">
          <div>
            <h3>{title}</h3>
            {subtitle && <p>{subtitle}</p>}
          </div>
          <button type="button" className="icon-btn" onClick={onClose} aria-label="Tutup">
            <X size={17} />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  );
}
