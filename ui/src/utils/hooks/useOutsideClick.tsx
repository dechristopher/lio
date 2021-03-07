/* istanbul ignore file */
import {RefObject, useEffect} from "react";

type Event = MouseEvent | TouchEvent;
type SingleOrArray<T> = T | T[]

//@TODO: Test this hook

/**
 * The hook useOutsideClick is a generic shared React Hook that takes in a ref, or array of refs,
 * and fires off a handler function when a click is executed outside of the bounds of any of the ref(s).
 *
 * @see https://usehooks-typescript.com/react-hook/use-on-click-outside
 *
 * @template T
 * @param {SingleOrArray<RefObject<T>>} ref - The bounding ref(s)
 * @param {(event: Event) => void} handler - The function to be executed upon clicking outside
 *
 * @example
 * useOutsideClick([buttonRef], () => console.log("I clicked outside the button"))
 */
export function useOutsideClick<T extends HTMLElement = HTMLElement>(
	ref: SingleOrArray<RefObject<T>>,
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