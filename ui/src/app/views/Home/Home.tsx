import React, { FC } from "react";

export const Home: FC = () => {
	return (
		<div className="chin">
			<div className="quick_game_button_group">
				<div className="text-xl italic font-bold text-center">
					Quick Game
				</div>
				<div className="flex items-center">
					<button
						className="libtn"
						title="Quick game vs human"
						aria-label="Quick game versus human"
					>
						👶
					</button>

					<div className="text-2xl mx-1.5">or</div>
					<button
						className="libtn"
						title="Quick game vs the computer"
						aria-label="Quick game versus the computer"
					>
						🤖
					</button>
				</div>
			</div>
			<div className="cgbtn-container">
				<button
					className="libtn cgbtn"
					title="Create a custom game"
					aria-label="Create a custom game"
				>
					Create Game
				</button>
			</div>
		</div>
	);
};
