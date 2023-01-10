"use client";

import React, { FC, Fragment, useEffect, useState } from "react";
import { Dialog, Transition } from "@headlessui/react";
import { XMarkIcon } from "@heroicons/react/24/outline";
import styles from "./CustomGameModal.module.scss";
import classNames from "classnames";
import Button from "../components/Button/Button";
import { useRouter } from "next/navigation";
import {
	NewCustomRoomPayload,
	PlayerColor,
	Variant,
	VariantGroup,
	VariantPools,
} from "@client/proto/ws_pb";
import { Piece, PieceType, SplitPiece } from "@client/components/Piece/Piece";

interface CreateGameModalProps {
	open: boolean;
	close: () => void;
}

const CreateGameModal: FC<CreateGameModalProps> = (props) => {
	const router = useRouter();
	const [variantPools, setVariantPools] = useState<VariantPools | null>(null);
	const [selectedVariant, setSelectedVariant] = useState<Variant | null>(
		null,
	);
	const [selectedColor, setSelectedColor] = useState<PlayerColor | null>(
		null,
	);

	useEffect(() => {
		fetch("/api/pools").then(async (res) => {
			if (res.status === 200) {
				const data = await res.json();
				setVariantPools(VariantPools.fromJson(data));
			}
		});
	}, []);

	useEffect(() => {
		if (!!selectedVariant && selectedColor !== null) {
			fetch("/api/room/new/human", {
				method: "POST",
				headers: {
					Accept: "application/json",
					"Content-Type": "application/json",
				},
				body: new NewCustomRoomPayload({
					playerColor: selectedColor,
					variantHtmlName: selectedVariant.htmlName,
				}).toBinary(),
			}).then((response) => {
				if (response.status === 200) {
					router.push(response.url);
				} else {
					console.log("Error");
					// TODO handle error
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
	ratingPools: VariantPools | null;
	selectedVariant: Variant | null;
	setVariant: (variant: Variant) => void;
}

const VariantSelector = (props: VariantSelectorProps) => {
	return (
		<div className={styles.variantBtnContainer}>
			{props.ratingPools?.pools &&
				Object.values(props.ratingPools.pools).map((poolGroup) =>
					poolGroup.variants.map((pool) => (
						<Button
							key={`${pool.group}-${pool.htmlName}`}
							onClick={() => props.setVariant(pool)}
							className={classNames([
								styles.variantBtn,
								{
									[styles.selected]:
										pool.name ===
										props.selectedVariant?.name,
								},
							])}
						>
							<div className="text-lg">{pool.name}</div>
							<div className="italic">
								{VariantGroup[pool.group].toLowerCase()}
							</div>
						</Button>
					)),
				)}
		</div>
	);
};

interface ColorSelectorProps {
	disabled: boolean;
	setColor: (color: PlayerColor) => void;
}

const ColorSelector = (props: ColorSelectorProps) => {
	return (
		<div className="flex gap-x-2 items-center justify-center py-4">
			<Button
				disabled={props.disabled}
				title="Play the white pieces first"
				className={classNames(styles.colorSelectBtn, {
					[styles.disabled]: props.disabled,
				})}
				onClick={() => props.setColor(PlayerColor.WHITE)}
			>
				<Piece
					pieceType={PieceType.KING}
					pieceColor={PlayerColor.WHITE}
				/>
			</Button>

			<Button
				disabled={props.disabled}
				title="Play either set of pieces first"
				className={classNames(styles.colorSelectBtn, styles.big, {
					[styles.disabled]: props.disabled,
				})}
				onClick={() => {
					props.setColor(PlayerColor.UNSPECIFIED);
				}}
			>
				<SplitPiece />
			</Button>

			<Button
				disabled={props.disabled}
				title="Play the black pieces first"
				className={classNames(styles.colorSelectBtn, {
					[styles.disabled]: props.disabled,
				})}
				onClick={() => props.setColor(PlayerColor.BLACK)}
			>
				<Piece
					pieceType={PieceType.KING}
					pieceColor={PlayerColor.BLACK}
				/>
			</Button>
		</div>
	);
};

export default CreateGameModal;
