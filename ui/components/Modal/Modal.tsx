import { Transition, Dialog } from "@headlessui/react";
import { XMarkIcon } from "@heroicons/react/24/outline";
import { Fragment, ReactNode } from "react";
import styles from "./Modal.module.scss";

export interface ModalProps {
	open: boolean;
	close: () => void;
	children: ReactNode;
}

export function Modal(props: ModalProps) {
	return (
		<Transition.Root show={props.open} as={Fragment}>
			<Dialog as="div" className="relative z-10" onClose={props.close}>
				<Transition.Child
					as={Fragment}
					enter="ease-out duration-300"
					enterFrom="opacity-0"
					enterTo="opacity-100"
					leave="ease-in duration-200"
					leaveFrom="opacity-100"
					leaveTo="opacity-0"
				>
					<div
						// cannot use a module class here because an invisible shade remains after the dialog closes
						className="fixed inset-0 transition-opacity"
						style={{
							backgroundColor: "rgba(0, 0, 0, 0.5)",
						}}
					/>
				</Transition.Child>

				<div className="fixed inset-0 z-10 overflow-y-auto">
					<div className="flex min-h-full justify-center p-4 text-center items-center sm:p-0">
						<Transition.Child
							as={Fragment}
							enter="ease-out duration-300"
							enterFrom="opacity-0 translate-y-4 sm:translate-y-0 sm:scale-95"
							enterTo="opacity-100 translate-y-0 sm:scale-100"
							leave="ease-in duration-200"
							leaveFrom="opacity-100 translate-y-0 sm:scale-100"
							leaveTo="opacity-0 translate-y-4 sm:translate-y-0 sm:scale-95"
						>
							<Dialog.Panel className={styles.body}>
								<button
									className={styles.close}
									onClick={() => props.close()}
								>
									<span className="sr-only">Close</span>
									<XMarkIcon
										className="h-5 w-5"
										aria-hidden="true"
										strokeWidth="4"
									/>
								</button>

								{props.children}
							</Dialog.Panel>
						</Transition.Child>
					</div>
				</div>
			</Dialog>
		</Transition.Root>
	);
}
