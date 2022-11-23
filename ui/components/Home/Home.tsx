import React, { useState } from "react";
import Button from "../Button/Button";
import { useRouter } from "next/router";
// import CreateGameModal from "../../app/CustomGameModal";

export default function Home() {
	const router = useRouter();
	const [modalOpen, setModalOpen] = useState(false);

	return (
		<div className="flex flex-col items-center">
			<div className="text-xl italic font-bold">QUICK GAME</div>

			<div className="flex items-center">
				<Button
					className="text-2xl leading-none"
					title="Quick game vs human"
					aria-label="Quick game versus human"
					onClick={() => {
						// TODO do something
					}}
				>
					👶
				</Button>

				<div className="text-2xl font-bold mx-1">or</div>

				<Button
					className="text-2xl leading-none"
					title="Quick game vs the computer"
					aria-label="Quick game versus the computer"
					onClick={() => {
						fetch("api/room/new/computer").then((response) => {
							if (response.status === 200) {
								router.push(response.url);
							}
						});
					}}
				>
					🤖
				</Button>
			</div>

			<div className="line-break" />

			<Button
				className="font-semibold mx-3"
				style={{
					paddingBottom:
						"8px" /** this is inlined to override the padding value from the Button style */,
				}}
				title="Create a custom game"
				aria-label="Create a custom game"
				onClick={() => setModalOpen(true)}
			>
				CREATE GAME
			</Button>

			{/* <CreateGameModal
				open={modalOpen}
				close={() => setModalOpen(false)}
			/> */}
		</div>
	);
}
