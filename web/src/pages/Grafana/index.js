import React from 'react';

const Grafana = () => {
  // 使用相对路径，这样cookie可以正常传递到后端代理
  // 后端代理会将请求转发到Grafana服务器
  const grafanaUrl = 'https://www.furion-tech.com/api/grafana/login';
//    const grafanaUrl = 'https://test.furion-tech.com:82/api/grafana/login';

  return (
    <div style={{ width: '100%', height: '100vh', overflow: 'hidden' }}>
      <iframe
        src={grafanaUrl}
        style={{
          width: '100%',
          height: '100%',
          border: 'none',
          display: 'block'
        }}
        title="Grafana Dashboard"
        allow="fullscreen"
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox"
      />
    </div>
  );
};

export default Grafana;

