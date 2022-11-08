import React, { FC, Fragment, useEffect, useState } from "react";
import { Dialog, Transition } from "@headlessui/react";
import { XMarkIcon } from "@heroicons/react/24/outline";
import styles from "./CreateModal.module.scss";
import classNames from "classnames";
import Image from "next/image";
import Button from "../Button/Button";
import { Color } from "@/proto/proto";
import { useRouter } from "next/router";
import { VariantPools, Variant } from "@/proto/pools";

type NewGamePayload = {
	"time-control": string;
	color: "w" | "b";
};

interface CreateGameModalProps {
	open: boolean;
	close: () => void;
}

const CreateGameModal: FC<CreateGameModalProps> = (props) => {
	const router = useRouter();
	const [variantPools, setVariantPools] = useState<VariantPools>({});
	const [selectedVariant, setSelectedVariant] = useState<Variant | null>(
		null,
	);
	const [selectedColor, setSelectedColor] = useState<Color | null>(null);

	useEffect(() => {
		fetch("/api/pools")
			.then((res) => res.json())
			.then((data) => setVariantPools(data));
	}, []);

	useEffect(() => {
		if (!!selectedVariant && !!selectedColor) {
			const payload: NewGamePayload = {
				"time-control": selectedVariant.html_name,
				color: selectedColor === Color.WHITE ? "w" : "b",
			};
			fetch("/api/new/human", {
				method: "POST",
				headers: {
					Accept: "application/json",
					"Content-Type": "application/json",
				},
				body: JSON.stringify(payload),
			}).then((response) => {
				if (response.status === 200) {
					router.push(response.url);
				}
			});
		}
	}, [selectedVariant, selectedColor, router]);

	const handleClose = () => {
		setSelectedColor(null);
		setSelectedVariant(null);
		props.close();
	};

	return (
		<Transition.Root show={props.open} as={Fragment}>
			<Dialog as="div" className="relative z-10" onClose={handleClose}>
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
									onClick={handleClose}
								>
									<span className="sr-only">Close</span>
									<XMarkIcon
										className="h-5 w-5"
										aria-hidden="true"
										strokeWidth="4"
									/>
								</button>

								<div className="font-bold leading-6 text-center text-2xl mt-1">
									Create a game
								</div>

								<VariantSelector
									ratingPools={variantPools}
									selectedVariant={selectedVariant}
									setVariant={setSelectedVariant}
								/>

								<ColorSelector
									disabled={!selectedVariant}
									setColor={setSelectedColor}
								/>
							</Dialog.Panel>
						</Transition.Child>
					</div>
				</div>
			</Dialog>
		</Transition.Root>
	);
};

interface VariantSelectorProps {
	ratingPools: VariantPools;
	selectedVariant: Variant | null;
	setVariant: (variant: Variant) => void;
}

const VariantSelector = (props: VariantSelectorProps) => {
	return (
		<div className={styles.variantBtnContainer}>
			{Object.values(props.ratingPools).map((poolGroup) =>
				poolGroup.map((pool) => (
					<Button
						key={`${pool.group}-${pool.html_name}`}
						onClick={() => props.setVariant(pool)}
						className={classNames([
							styles.variantBtn,
							{
								[styles.selected]:
									pool.name === props.selectedVariant?.name,
							},
						])}
					>
						<div className="text-lg">{pool.name}</div>
						<div className="italic">{pool.group}</div>
					</Button>
				)),
			)}
		</div>
	);
};

interface ColorSelectorProps {
	disabled: boolean;
	setColor: (color: Color) => void;
}

const ColorSelector = (props: ColorSelectorProps) => {
	return (
		<div className="flex gap-x-2 items-center justify-center py-4">
			<Button
				disabled={props.disabled}
				className={classNames(styles.colorSelectBtn, {
					[styles.disabled]: props.disabled,
				})}
				onClick={() => props.setColor(Color.WHITE)}
			>
				<Image
					width="48px"
					height="48px"
					src="/images/pieces/wK.svg"
					alt="Play the white pieces first"
				/>
			</Button>

			<Button
				disabled={props.disabled}
				className={classNames(styles.colorSelectBtn, styles.big, {
					[styles.disabled]: props.disabled,
				})}
				onClick={() => {
					if (Math.random() > 0.5) {
						props.setColor(Color.WHITE);
					} else {
						props.setColor(Color.BLACK);
					}
				}}
			>
				<Image
					width="48px"
					height="48px"
					src="/images/pieces/wbK.svg"
					alt="Play either set of pieces first"
				/>
			</Button>

			<Button
				disabled={props.disabled}
				className={classNames(styles.colorSelectBtn, {
					[styles.disabled]: props.disabled,
				})}
				onClick={() => props.setColor(Color.BLACK)}
			>
				<Image
					width="48px"
					height="48px"
					src="/images/pieces/bK.svg"
					alt="Play the black pieces first"
				/>
			</Button>
		</div>
	);
};

export default CreateGameModal;
