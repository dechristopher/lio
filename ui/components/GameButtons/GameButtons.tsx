import React from "react";
import Link from "next/link";
import {
	CreateGameButtonStyle,
	QuickGameButtonGroup,
	Break,
	QuickGameButtonStyle,
} from "./GameButtons.css";
import Button from "../Button/Button";
import { useRouter } from "next/router";

export default function GameButtons(): JSX.Element {
	const router = useRouter();

	return (
		<div>
			<div className={QuickGameButtonGroup}>
				<div className="text-xl italic font-bold text-center">
					QUICK GAME
				</div>
				<div className="flex items-center">
					{/* TODO update href */}
					<Link href="game/123">
						<a>
							<Button
								className={QuickGameButtonStyle}
								title="Quick game vs human"
								aria-label="Quick game versus human"
							>
								👶
							</Button>
						</a>
					</Link>

					<div className="text-2xl font-bold mx-1">or</div>

					{/* <Link href="/game">
						<a> */}
					<Button
						className={QuickGameButtonStyle}
						title="Quick game vs the computer"
						aria-label="Quick game versus the computer"
						onClick={() => {
							fetch("api/new/computer").then((response) => {
								console.log(response);
								if (response.status === 200) {
									router.push(response.url);
								}
							});
						}}
					>
						🤖
					</Button>
					{/* </a>
					</Link> */}
				</div>
			</div>

			<div className="flex flex-col">
				<div className={Break} />

				<Button
					title="Create a custom game"
					className={CreateGameButtonStyle}
					aria-label="Create a custom game"
				>
					<div>CREATE GAME</div>
				</Button>
			</div>
		</div>
	);
}
