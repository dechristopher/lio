import React from "react";
import Link from "next/link";
import {
	CreateGameButtonStyle,
	Chin,
	QuickGameButtonStyle,
} from "./CreateGameButtons.css";
import Button from "../Button/Button";

export default function CreateGameButtons() {
	return (
		<div className={Chin}>
			<div className="quick_game_button_group">
				<div className="text-xl italic font-bold text-center">
					Quick Game
				</div>
				<div className="flex items-center">
					<Link href="/game">
						<Button
							className={QuickGameButtonStyle}
							title="Quick game vs human"
							aria-label="Quick game versus human"
						>
							👶
						</Button>
					</Link>

					<div className="text-2xl mx-1.5">or</div>

					<Link href="/game">
						<Button
							className={QuickGameButtonStyle}
							title="Quick game vs the computer"
							aria-label="Quick game versus the computer"
						>
							🤖
						</Button>
					</Link>
				</div>
			</div>
			<div>
				<Button
					className={CreateGameButtonStyle}
					title="Create a custom game"
					aria-label="Create a custom game"
				>
					Create Game
				</Button>
			</div>
		</div>
	);
}
