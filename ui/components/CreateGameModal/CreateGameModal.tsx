import React, { FC, Fragment, useEffect, useRef, useState } from "react";
import { Dialog, Transition } from "@headlessui/react";
import {
	ExclamationTriangleIcon,
	XMarkIcon,
} from "@heroicons/react/24/outline";
import styles from "./CreateModal.module.scss";
import classNames from "classnames";
import Image from "next/image";
import Button from "../Button/Button";
import { Color } from "@/proto/proto";
import { useRouter } from "next/router";
import axios, { AxiosResponse } from "axios";
import useSWR, { Fetcher } from "swr";

enum RatingGroup {
	Bullet = "bullet",
	Blitz = "blitz",
	Rapid = "rapid",
	Hyper = "hyper",
	Ulti = "ulti",
}

type RatingPoolTime = {
	t: number;
	i: number;
	d: number;
};

type RatingPool = {
	name: string;
	html_name: string;
	group: RatingGroup;
	time: RatingPoolTime;
};

type RatingPoolResponse = Partial<
	Record<keyof typeof RatingGroup, RatingPool[]>
>;

interface CreateGameModalProps {
	open: boolean;
	close: () => void;
}

type CustomGamePayload = {
	"time-control": string;
	color: "w" | "b";
};

const fetcher: Fetcher<RatingPoolResponse, string> = (url) =>
	fetch(url).then((res) => res.json());

const CreateGameModal: FC<CreateGameModalProps> = (props) => {
	const router = useRouter();
	const [selectedRating, setSelectedRating] = useState<RatingPool | null>(
		null,
	);
	const [selectedColor, setSelectedColor] = useState<Color | null>(null);

	// TODO handle errors
	const ratingPoolsReq = useSWR("/api/pools", fetcher);

	useEffect(() => {
		if (!!selectedRating && !!selectedColor) {
			const payload: CustomGamePayload = {
				"time-control": selectedRating.html_name,
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
	}, [selectedRating, selectedColor]);

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
					<div className="fixed inset-0 bg-gray-500 bg-opacity-75 transition-opacity" />
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
							<Dialog.Panel
								className="relative transform rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 sm:max-w-lg"
								style={{
									borderRadius: "3px",
									backgroundColor: "#cca57b",
								}}
							>
								<button
									className={styles.close}
									onClick={props.close}
									autoFocus={true}
								>
									<span className="sr-only">Close</span>
									<XMarkIcon
										className="h-5 w-5"
										aria-hidden="true"
										strokeWidth="4"
									/>
								</button>

								<div>
									{/* <div className="mx-auto flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full bg-red-100 sm:mx-0 sm:h-10 sm:w-10">
										<ExclamationTriangleIcon
											className="h-6 w-6 text-red-600"
											aria-hidden="true"
										/>
									</div> */}
									<div className="text-center sm:text-left w-100">
										<Dialog.Title
											as="h3"
											className="font-bold leading-6 text-gray-900 text-center"
											style={{
												fontSize: "1.5rem",
											}}
										>
											Create a game
										</Dialog.Title>
										<div
											className="mt-1.5"
											style={{
												display: "grid",
												gridTemplateColumns:
													"auto auto auto",
												alignItems: "center",
												columnGap: 8,
												rowGap: 8,
												backgroundColor: "#f1d8b8",
												borderTop: "1px solid #8c6d54",
												borderBottom:
													"1px solid #8c6d54",
												padding: "12px 24px",
											}}
										>
											{Object.values(
												ratingPoolsReq.data ?? {},
											).map((poolGroup) =>
												poolGroup.map((pool) => (
													<RatingPoolTile
														key={`${pool.group}-${pool.html_name}`}
														ratingPool={pool}
														selectedRating={
															selectedRating
														}
														selectRating={
															setSelectedRating
														}
													/>
												)),
											)}
										</div>
									</div>

									<div
										className="flex"
										style={{
											columnGap: 8,
											alignItems: "center",
											justifyContent: "center",
											padding: "16px 0",
										}}
									>
										<Button
											style={{
												height: "48px",
												width: "64px",
											}}
										>
											<Image
												width="100%"
												height="100%"
												src="/images/pieces/wK.svg"
												alt="Play the white pieces first"
												onClick={() =>
													setSelectedColor(
														Color.WHITE,
													)
												}
											/>
										</Button>

										<Button
											style={{
												height: "64px",
												width: "80px",
											}}
										>
											<Image
												width="100%"
												height="100%"
												src="/images/pieces/wbK.svg"
												alt="Play either set of pieces first"
												onClick={() => {
													if (Math.random() > 0.5) {
														setSelectedColor(
															Color.WHITE,
														);
													} else {
														setSelectedColor(
															Color.BLACK,
														);
													}
												}}
											/>
										</Button>

										<Button
											style={{
												height: "48px",
												width: "64px",
											}}
										>
											<Image
												width="100%"
												height="100%"
												src="/images/pieces/bK.svg"
												alt="Play the black pieces first"
												onClick={() =>
													setSelectedColor(
														Color.BLACK,
													)
												}
											/>
										</Button>
									</div>
									{/* TODO add color selection buttons */}
								</div>

								{/* <div className="mt-5 sm:mt-4 sm:flex sm:flex-row-reverse">
									<button
										type="button"
										className="inline-flex w-full justify-center rounded-md border border-transparent bg-red-600 px-4 py-2 text-base font-medium text-white shadow-sm hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2 sm:ml-3 sm:w-auto sm:text-sm"
										onClick={props.close}
									>
										Deactivate
									</button>
									<button
										type="button"
										className="mt-3 inline-flex w-full justify-center rounded-md border border-gray-300 bg-white px-4 py-2 text-base font-medium text-gray-700 shadow-sm hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-2 sm:mt-0 sm:w-auto sm:text-sm"
										onClick={props.close}
									>
										Cancel
									</button>
								</div> */}
							</Dialog.Panel>
						</Transition.Child>
					</div>
				</div>
			</Dialog>
		</Transition.Root>
	);
};

interface RatingPoolTileProps {
	ratingPool: RatingPool;
	selectedRating: RatingPool | null;
	selectRating: (rating: RatingPool) => void;
}

const RatingPoolTile: FC<RatingPoolTileProps> = (props) => {
	return (
		<button
			style={{
				display: "flex",
				flexDirection: "column",
				lineHeight: 0.75,
			}}
			className={classNames([
				styles.tcBox,
				{
					[styles.selected]:
						props.ratingPool.name === props.selectedRating?.name,
				},
			])}
			onClick={() => props.selectRating(props.ratingPool)}
		>
			<div className="text-lg">{props.ratingPool.name}</div>
			<div className="italic">{props.ratingPool.group}</div>
		</button>
	);
};

export default CreateGameModal;
