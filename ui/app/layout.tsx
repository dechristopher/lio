import { Footer } from "@components/Footer/Footer";
import { Header } from "@components/Header/Header";
import styles from "./HomePage.module.scss";
import "../styles/global.scss";

export default function RootLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<html>
			<head />
			<body>
				<div className="flex flex-col items-center pt-8">
					<Header />
					<div className={styles.body}>{children}</div>
					<Footer />
				</div>
			</body>
		</html>
	);
}
