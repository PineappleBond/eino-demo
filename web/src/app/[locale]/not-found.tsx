import { Result, Button } from 'antd';
import Link from 'next/link';

export default function NotFound() {
  return (
    <div style={{ display: 'flex', justifyContent: 'center', padding: '80px 24px' }}>
      <Result
        status="404"
        title="404"
        subTitle="Sorry, the page you visited does not exist."
        extra={
          <Button type="primary">
            <Link href="/">Back Home</Link>
          </Button>
        }
      />
    </div>
  );
}
