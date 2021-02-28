import {RefObject, useEffect} from "react";

type Event = MouseEvent | TouchEvent;

export function useOutsideClick<T extends HTMLElement = HTMLElement>(
	ref: RefObject<T> | RefObject<T>[],
	handler: (event: Event) => void
): void {
	useEffect(() => {
		const listener = (event: Event) => {
			if (Array.isArray(ref)) {
				for (const refObj of ref) {
					const element: T | null = refObj?.current;

					// if the element being clicked is contained by the ref or the
					// ref is not currently being rendered, do nothing.
					if (!element || element.contains(event.target as Node || null)) {
						return;
					}
				}
			} else {
				const element: T | null = ref?.current;

				// if the element being clicked is contained by the ref or the
				// ref is not currently being rendered, do nothing.
				if (!element || element.contains(event.target as Node || null)) {
					return;
				}
			}

			handler(event);
		}

		document.addEventListener(`mousedown`, listener)
		document.addEventListener(`touchstart`, listener)
		return () => {
			document.removeEventListener(`mousedown`, listener)
			document.removeEventListener(`touchstart`, listener)
		}
	}, [ref, handler])
}